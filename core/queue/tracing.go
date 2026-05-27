package queue

import (
	"context"

	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracecontext"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"
)

func CorrelationHeadersFromContext(ctx context.Context) map[string]string {
	return tracecontext.HeadersFromContext(ctx)
}

func ContextFromCorrelationHeaders(ctx context.Context, headers map[string]string) context.Context {
	return tracecontext.ContextFromHeaders(ctx, headers)
}

func CorrelationHeaderValue(headers map[string]string, key string) string {
	return tracecontext.HeaderValue(headers, key)
}

func TrackIDFromHeaders(headers map[string]string) string {
	return CorrelationHeaderValue(headers, tracking.Header)
}

func RequestIDFromHeaders(headers map[string]string) string {
	return CorrelationHeaderValue(headers, requestid.Header)
}

func TraceIDFromHeaders(headers map[string]string) string {
	return tracecontext.TraceIDFromHeaders(headers)
}

func SpanIDFromHeaders(headers map[string]string) string {
	return tracecontext.SpanIDFromHeaders(headers)
}

func SetSpanCorrelationAttributes(ctx context.Context, span tracing.Span) {
	tracing.SetSpanCorrelationAttributes(ctx, span)
}
