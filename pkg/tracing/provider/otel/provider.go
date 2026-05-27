//go:build sdkit_tracing

package otel

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/huwenlong92/sdkit/core/tracecontext"
	pkgtracing "github.com/huwenlong92/sdkit/pkg/tracing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func init() {
	pkgtracing.RegisterProvider("otel", provider{})
}

type provider struct{}

func (provider) Init(ctx context.Context, cfg pkgtracing.Config) (func(context.Context) error, error) {
	cfg = pkgtracing.NormalizeConfig(cfg)
	otel.SetTextMapPropagator(newPropagator())
	if !cfg.Enabled {
		return noopShutdown, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	exporterCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithTimeout(cfg.Timeout),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(exporterCtx, opts...)
	if err != nil {
		if cfg.Strict {
			return nil, err
		}
		return noopShutdown, nil
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			"",
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("deployment.environment", cfg.Environment),
		),
	)
	if err != nil {
		if cfg.Strict {
			return nil, err
		}
		return noopShutdown, nil
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	)
	otel.SetTracerProvider(tracerProvider)
	return tracerProvider.Shutdown, nil
}

func (provider) StartSpan(ctx context.Context, name string, opts pkgtracing.SpanOptions, attrs ...pkgtracing.Attr) (context.Context, pkgtracing.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	tracerName := opts.TracerName
	if tracerName == "" {
		tracerName = "sdkitgo/core/tracing"
	}
	startOpts := make([]oteltrace.SpanStartOption, 0, 2)
	if opts.Kind != "" {
		startOpts = append(startOpts, oteltrace.WithSpanKind(spanKind(opts.Kind)))
	}
	if len(attrs) > 0 {
		startOpts = append(startOpts, oteltrace.WithAttributes(convertAttrs(attrs)...))
	}
	ctx, span := otel.Tracer(tracerName).Start(ctx, name, startOpts...)
	if !span.SpanContext().IsValid() {
		traceID := tracecontext.TraceID(ctx)
		if traceID == "" {
			traceID = randomHex(16)
		}
		spanID := randomHex(8)
		ctx = tracecontext.ContextWithTrace(ctx, traceID, spanID)
		return ctx, fallbackSpan{traceID: traceID, spanID: spanID}
	}
	ctx = contextWithSpan(ctx, span)
	return ctx, spanWrapper{span: span}
}

func (provider) InjectCarrier(ctx context.Context, carrier tracecontext.Carrier) {
	if ctx == nil || carrier == nil {
		return
	}
	otel.GetTextMapPropagator().Inject(ctx, carrierAdapter{Carrier: carrier})
	tracecontext.InjectCarrier(ctx, carrier)
}

func (provider) ExtractCarrier(ctx context.Context, carrier tracecontext.Carrier) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if carrier == nil {
		return ctx
	}
	ctx = otel.GetTextMapPropagator().Extract(ctx, carrierAdapter{Carrier: carrier})
	ctx = contextWithSpanContext(ctx, oteltrace.SpanContextFromContext(ctx))
	return tracecontext.ExtractCarrier(ctx, carrier)
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func noopShutdown(context.Context) error {
	return nil
}

type carrierAdapter struct {
	tracecontext.Carrier
}

func (c carrierAdapter) Keys() []string {
	keyCarrier, ok := c.Carrier.(tracecontext.KeyCarrier)
	if !ok {
		return nil
	}
	return keyCarrier.Keys()
}

type spanWrapper struct {
	span oteltrace.Span
}

func (s spanWrapper) End() {
	if s.span != nil {
		s.span.End()
	}
}

func (s spanWrapper) IsRecording() bool {
	return s.span != nil && s.span.IsRecording()
}

func (s spanWrapper) RecordError(err error) {
	if s.span != nil && err != nil {
		s.span.RecordError(err)
	}
}

func (s spanWrapper) SetStatus(code pkgtracing.StatusCode, message string) {
	if s.span == nil {
		return
	}
	switch code {
	case pkgtracing.StatusOK:
		s.span.SetStatus(codes.Ok, message)
	case pkgtracing.StatusError:
		s.span.SetStatus(codes.Error, message)
	default:
		s.span.SetStatus(codes.Unset, message)
	}
}

func (s spanWrapper) SetAttributes(attrs ...pkgtracing.Attr) {
	if s.span == nil || len(attrs) == 0 {
		return
	}
	s.span.SetAttributes(convertAttrs(attrs)...)
}

func (s spanWrapper) TraceID() string {
	if s.span == nil {
		return ""
	}
	spanCtx := s.span.SpanContext()
	if !spanCtx.IsValid() || !spanCtx.HasTraceID() {
		return ""
	}
	return spanCtx.TraceID().String()
}

func (s spanWrapper) SpanID() string {
	if s.span == nil {
		return ""
	}
	spanCtx := s.span.SpanContext()
	if !spanCtx.IsValid() || !spanCtx.HasSpanID() {
		return ""
	}
	return spanCtx.SpanID().String()
}

func spanKind(kind pkgtracing.SpanKind) oteltrace.SpanKind {
	switch kind {
	case pkgtracing.SpanKindServer:
		return oteltrace.SpanKindServer
	case pkgtracing.SpanKindProducer:
		return oteltrace.SpanKindProducer
	case pkgtracing.SpanKindConsumer:
		return oteltrace.SpanKindConsumer
	default:
		return oteltrace.SpanKindInternal
	}
}

func convertAttrs(attrs []pkgtracing.Attr) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		if attr.Key == "" {
			continue
		}
		switch value := attr.Value.(type) {
		case string:
			out = append(out, attribute.String(attr.Key, value))
		case int:
			out = append(out, attribute.Int(attr.Key, value))
		case int64:
			out = append(out, attribute.Int64(attr.Key, value))
		case float64:
			out = append(out, attribute.Float64(attr.Key, value))
		case bool:
			out = append(out, attribute.Bool(attr.Key, value))
		default:
			out = append(out, attribute.String(attr.Key, ""))
		}
	}
	return out
}

func contextWithSpan(ctx context.Context, span oteltrace.Span) context.Context {
	if span == nil {
		return ctx
	}
	return contextWithSpanContext(ctx, span.SpanContext())
}

func contextWithSpanContext(ctx context.Context, spanCtx oteltrace.SpanContext) context.Context {
	if !spanCtx.IsValid() {
		return ctx
	}
	traceID := ""
	spanID := ""
	if spanCtx.HasTraceID() {
		traceID = spanCtx.TraceID().String()
	}
	if spanCtx.HasSpanID() {
		spanID = spanCtx.SpanID().String()
	}
	return tracecontext.ContextWithTrace(ctx, traceID, spanID)
}

type fallbackSpan struct {
	traceID string
	spanID  string
}

func (fallbackSpan) End() {}

func (fallbackSpan) IsRecording() bool {
	return false
}

func (fallbackSpan) RecordError(error) {}

func (fallbackSpan) SetStatus(pkgtracing.StatusCode, string) {}

func (fallbackSpan) SetAttributes(...pkgtracing.Attr) {}

func (s fallbackSpan) TraceID() string {
	return s.traceID
}

func (s fallbackSpan) SpanID() string {
	return s.spanID
}

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}
