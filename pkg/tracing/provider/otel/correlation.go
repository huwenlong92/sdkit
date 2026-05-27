//go:build sdkit_tracing_otel

package otel

import (
	"context"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/tracecontext"
	"github.com/huwenlong92/sdkit/core/tracking"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func setSpanCorrelationAttributes(ctx context.Context, span oteltrace.Span) {
	if span == nil || !span.IsRecording() {
		return
	}
	attrs := make([]attribute.KeyValue, 0, 6)
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
	}
	if requestID := tracecontext.RequestID(ctx); requestID != "" {
		attrs = append(attrs, attribute.String(logger.RequestIDKey, requestID))
	}
	if traceparent := tracecontext.Traceparent(ctx); traceparent != "" {
		attrs = append(attrs, attribute.String("traceparent", traceparent))
	}
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
}
