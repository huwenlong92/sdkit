package requestid

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	Header = "X-Request-ID"
	Key    = "request_id"
)

type contextKey struct{}

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

// Middleware passes through X-Request-ID or generates one when missing.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(Header)
		if id == "" {
			id = uuid.New().String()
		}
		c.Set(Key, id)
		c.Header(Header, id)
		ctx := WithRequestID(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func Get(c *gin.Context) string {
	id, _ := c.Get(Key)
	if id == nil {
		return ""
	}
	return id.(string)
}
