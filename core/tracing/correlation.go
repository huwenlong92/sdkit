package tracing

import (
	"context"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/tracecontext"
	"github.com/huwenlong92/sdkit/core/tracking"
)

func HeadersFromContext(ctx context.Context) map[string]string {
	return tracecontext.HeadersFromContext(ctx)
}

func ContextFromHeaders(ctx context.Context, headers map[string]string) context.Context {
	return tracecontext.ContextFromHeaders(ctx, headers)
}

func InjectCarrier(ctx context.Context, carrier tracecontext.Carrier) {
	tracecontext.InjectCarrier(ctx, carrier)
}

func ExtractCarrier(ctx context.Context, carrier tracecontext.Carrier) context.Context {
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

func SetCorrelationAttributes(ctx context.Context, span Span) {
	SetSpanCorrelationAttributes(ctx, span)
}

func SetSpanCorrelationAttributes(ctx context.Context, span Span) {
	setSpanCorrelationAttributes(ctx, span, false)
}

func SetHTTPSpanCorrelationAttributes(ctx context.Context, span Span) {
	setSpanCorrelationAttributes(ctx, span, true)
}

func setSpanCorrelationAttributes(ctx context.Context, span Span, includeLegacySD bool) {
	if span == nil || !span.IsRecording() {
		return
	}
	attrs := make([]Attr, 0, 8)
	if traceID := firstNonEmpty(span.TraceID(), tracecontext.TraceID(ctx)); traceID != "" {
		attrs = append(attrs, String(logger.TraceIDKey, traceID))
	}
	if spanID := firstNonEmpty(span.SpanID(), tracecontext.SpanID(ctx)); spanID != "" {
		attrs = append(attrs, String(logger.SpanIDKey, spanID))
	}
	if trackID := tracking.TrackID(ctx); trackID != "" {
		attrs = append(attrs, String(logger.TrackIDKey, trackID))
		if includeLegacySD {
			attrs = append(attrs, String("sd."+logger.TrackIDKey, trackID))
		}
	}
	if requestID := RequestID(ctx); requestID != "" {
		attrs = append(attrs, String(logger.RequestIDKey, requestID))
		if includeLegacySD {
			attrs = append(attrs, String("sd."+logger.RequestIDKey, requestID))
		}
	}
	if traceparent := Traceparent(ctx); traceparent != "" {
		attrs = append(attrs, String("traceparent", traceparent))
	}
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
