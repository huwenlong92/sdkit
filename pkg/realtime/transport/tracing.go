package transport

import (
	"context"

	"github.com/huwenlong92/sdkit/core/tracecontext"
)

func TraceID(ctx context.Context) string {
	return tracecontext.TraceID(ctx)
}

func HeadersFromContext(ctx context.Context) map[string]string {
	return tracecontext.HeadersFromContext(ctx)
}
