package tracing

import (
	"context"

	"github.com/huwenlong92/sdkit/core/tracecontext"

	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func HeadersFromContext(ctx context.Context) map[string]string {
	return tracecontext.HeadersFromContext(ctx)
}

func ContextFromHeaders(ctx context.Context, headers map[string]string) context.Context {
	return tracecontext.ContextFromHeaders(ctx, headers)
}

func InjectCarrier(ctx context.Context, carrier propagation.TextMapCarrier) {
	tracecontext.InjectCarrier(ctx, carrier)
}

func ExtractCarrier(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	return tracecontext.ExtractCarrier(ctx, carrier)
}

func RequestID(ctx context.Context) string {
	return tracecontext.RequestID(ctx)
}

func Traceparent(ctx context.Context) string {
	return tracecontext.Traceparent(ctx)
}

func HeaderValue(headers map[string]string, key string) string {
	return tracecontext.HeaderValue(headers, key)
}

func TraceIDFromHeaders(headers map[string]string) string {
	return tracecontext.TraceIDFromHeaders(headers)
}

func SpanIDFromHeaders(headers map[string]string) string {
	return tracecontext.SpanIDFromHeaders(headers)
}

func SetSpanCorrelationAttributes(ctx context.Context, span oteltrace.Span) {
	tracecontext.SetSpanCorrelationAttributes(ctx, span)
}
