package requestid

import (
	"context"

	"github.com/google/uuid"
)

const (
	Header = "X-Request-ID"
	Key    = "request_id"
)

type contextKey struct{}

func New() string {
	return uuid.New().String()
}

// WithRequestID stores a request id in context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, requestID)
}

// FromContext returns the request id stored in context.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if requestID, ok := ctx.Value(contextKey{}).(string); ok && requestID != "" {
		return requestID
	}
	return ""
}
