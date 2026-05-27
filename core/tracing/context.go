package tracing

import (
	"context"

	"github.com/huwenlong92/sdkit/core/tracecontext"
)

func TraceID(ctx context.Context) string {
	return tracecontext.TraceID(ctx)
}

func SpanID(ctx context.Context) string {
	return tracecontext.SpanID(ctx)
}
