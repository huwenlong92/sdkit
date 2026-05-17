package transport

import (
	"context"

	"github.com/huwenlong92/sdkit/core/tracing"
)

func TraceID(ctx context.Context) string {
	return tracing.TraceID(ctx)
}

func HeadersFromContext(ctx context.Context) map[string]string {
	return tracing.HeadersFromContext(ctx)
}
