package tracing

import (
	"context"
	"strings"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracking"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func HeadersFromContext(ctx context.Context) map[string]string {
	if ctx == nil {
		return nil
	}
	headers := map[string]string{}
	InjectCarrier(ctx, propagation.MapCarrier(headers))
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func ContextFromHeaders(ctx context.Context, headers map[string]string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(headers) == 0 {
		return ctx
	}
	return ExtractCarrier(ctx, propagation.MapCarrier(headers))
}

func InjectCarrier(ctx context.Context, carrier propagation.TextMapCarrier) {
	if ctx == nil || carrier == nil {
		return
	}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	if trackID := tracking.TrackID(ctx); trackID != "" {
		carrier.Set(tracking.Header, trackID)
	}
	if requestID := RequestID(ctx); requestID != "" {
		carrier.Set(requestid.Header, requestID)
	}
}

func ExtractCarrier(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if carrier == nil {
		return ctx
	}
	ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
	if trackID := carrierValue(carrier, tracking.Header); trackID != "" {
		ctx = tracking.WithTrackID(ctx, trackID)
	}
	if requestID := carrierValue(carrier, requestid.Header); requestID != "" {
		ctx = requestid.WithRequestID(ctx, requestID)
	}
	headers := carrierFields(carrier)
	if traceID := TraceIDFromHeaders(headers); traceID != "" {
		ctx = logger.WithField(ctx, logger.TraceIDKey, traceID)
	}
	if spanID := SpanIDFromHeaders(headers); spanID != "" {
		ctx = logger.WithField(ctx, logger.SpanIDKey, spanID)
	}
	return ctx
}

func carrierValue(carrier propagation.TextMapCarrier, key string) string {
	if carrier == nil || key == "" {
		return ""
	}
	if value := carrier.Get(key); value != "" {
		return value
	}
	keyCarrier, ok := carrier.(interface{ Keys() []string })
	if !ok {
		return ""
	}
	for _, field := range keyCarrier.Keys() {
		if strings.EqualFold(field, key) {
			return carrier.Get(field)
		}
	}
	return ""
}

func RequestID(ctx context.Context) string {
	return requestid.FromContext(ctx)
}

func Traceparent(ctx context.Context) string {
	headers := HeadersFromContext(ctx)
	return HeaderValue(headers, "traceparent")
}

func HeaderValue(headers map[string]string, key string) string {
	if len(headers) == 0 || key == "" {
		return ""
	}
	if v := headers[key]; v != "" {
		return v
	}
	for k, v := range headers {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

func TraceIDFromHeaders(headers map[string]string) string {
	if traceID := HeaderValue(headers, logger.TraceIDKey); traceID != "" {
		return traceID
	}
	parts := strings.Split(HeaderValue(headers, "traceparent"), "-")
	if len(parts) >= 4 && len(parts[1]) == 32 {
		return parts[1]
	}
	return ""
}

func SpanIDFromHeaders(headers map[string]string) string {
	if spanID := HeaderValue(headers, logger.SpanIDKey); spanID != "" {
		return spanID
	}
	parts := strings.Split(HeaderValue(headers, "traceparent"), "-")
	if len(parts) >= 4 && len(parts[2]) == 16 {
		return parts[2]
	}
	return ""
}

func SetCorrelationAttributes(ctx context.Context) {
	SetSpanCorrelationAttributes(ctx, oteltrace.SpanFromContext(ctx))
}

func SetSpanCorrelationAttributes(ctx context.Context, span oteltrace.Span) {
	setSpanCorrelationAttributes(ctx, span, false)
}

func setSpanCorrelationAttributes(ctx context.Context, span oteltrace.Span, includeLegacySD bool) {
	if span == nil || !span.IsRecording() {
		return
	}
	attrs := make([]attribute.KeyValue, 0, 8)
	if spanCtx := span.SpanContext(); spanCtx.IsValid() {
		if spanCtx.HasTraceID() {
			attrs = append(attrs, attribute.String(logger.TraceIDKey, spanCtx.TraceID().String()))
		}
		if spanCtx.HasSpanID() {
			attrs = append(attrs, attribute.String(logger.SpanIDKey, spanCtx.SpanID().String()))
		}
	}
	if trackID := tracking.TrackID(ctx); trackID != "" {
		attrs = append(attrs, attribute.String(logger.TrackIDKey, trackID))
		if includeLegacySD {
			attrs = append(attrs, attribute.String("sd."+logger.TrackIDKey, trackID))
		}
	}
	if requestID := RequestID(ctx); requestID != "" {
		attrs = append(attrs, attribute.String(logger.RequestIDKey, requestID))
		if includeLegacySD {
			attrs = append(attrs, attribute.String("sd."+logger.RequestIDKey, requestID))
		}
	}
	if traceparent := Traceparent(ctx); traceparent != "" {
		attrs = append(attrs, attribute.String("traceparent", traceparent))
	}
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
}

func carrierFields(carrier propagation.TextMapCarrier) map[string]string {
	keyCarrier, ok := carrier.(interface{ Keys() []string })
	if !ok {
		fields := []string{"traceparent", logger.TraceIDKey, logger.SpanIDKey}
		headers := make(map[string]string, len(fields))
		for _, field := range fields {
			if value := carrier.Get(field); value != "" {
				headers[field] = value
			}
		}
		return headers
	}
	fields := keyCarrier.Keys()
	if len(fields) == 0 {
		return nil
	}
	headers := make(map[string]string, len(fields))
	for _, field := range fields {
		if value := carrier.Get(field); value != "" {
			headers[field] = value
		}
	}
	return headers
}
