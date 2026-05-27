package tracecontext

import (
	"context"

	oteltrace "go.opentelemetry.io/otel/trace"
)

func TraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	spanContext := oteltrace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() || !spanContext.HasTraceID() {
		return ""
	}
	return spanContext.TraceID().String()
}

func SpanID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	spanContext := oteltrace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() || !spanContext.HasSpanID() {
		return ""
	}
	return spanContext.SpanID().String()
}
