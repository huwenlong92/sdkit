package tracecontext

import (
	"context"

	"github.com/huwenlong92/sdkit/core/logger"
)

type contextKey string

const (
	traceIDKey contextKey = "trace_id"
	spanIDKey  contextKey = "span_id"
)

func ContextWithTrace(ctx context.Context, traceID string, spanID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if traceID != "" {
		ctx = context.WithValue(ctx, traceIDKey, traceID)
		ctx = logger.WithField(ctx, logger.TraceIDKey, traceID)
	}
	if spanID != "" {
		ctx = context.WithValue(ctx, spanIDKey, spanID)
		ctx = logger.WithField(ctx, logger.SpanIDKey, spanID)
	}
	return ctx
}

func TraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if traceID, ok := ctx.Value(traceIDKey).(string); ok {
		return traceID
	}
	return ""
}

func SpanID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if spanID, ok := ctx.Value(spanIDKey).(string); ok {
		return spanID
	}
	return ""
}
