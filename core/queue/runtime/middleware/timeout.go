package middleware

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"
)

func Timeout(defaultTimeout ...time.Duration) queue.Middleware {
	timeout := firstPositiveDuration(defaultTimeout...)
	return func(next queue.HandlerFunc) queue.HandlerFunc {
		return func(ctx context.Context, msg *queue.Message) error {
			d := timeout
			if d <= 0 {
				if metadataTimeout, ok := queue.MessageMetadataDuration(msg, queue.MessageMetadataTimeout); ok {
					d = metadataTimeout
				}
			}
			if d <= 0 {
				return next(ctx, msg)
			}
			if ctx == nil {
				ctx = context.Background()
			}
			timeoutCtx, cancel := context.WithTimeout(ctx, d)
			defer cancel()
			return next(timeoutCtx, msg)
		}
	}
}

func firstPositiveDuration(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
