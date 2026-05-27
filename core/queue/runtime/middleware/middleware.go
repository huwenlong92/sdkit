package middleware

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/tracing"
)

type RateLimitKeyFunc func(ctx context.Context, msg *queue.Message) (key string, limit int, window time.Duration, apply bool, err error)

func Tracing() queue.Middleware {
	return queue.ContextChain(func(c *queue.HandlerContext) error {
		msg := c.Message
		name := "handler::task"
		attrs := []tracing.Attr{
			tracing.String("worker.component", "queue"),
		}
		if msg != nil {
			name = "handler::" + msg.Type
			attrs = append(attrs,
				tracing.String("messaging.destination.name", msg.Queue),
				tracing.String("messaging.message.id", msg.ID),
				tracing.String("messaging.message.type", msg.Type),
				tracing.Int("messaging.message.retry_count", msg.RetryCount),
				tracing.Int("messaging.message.max_retry", msg.MaxRetry),
			)
		}

		ctx, span := tracing.StartSpanWithOptions(c.Context(), name, tracing.SpanOptions{
			TracerName: "sdkitgo/core/queue",
			Kind:       tracing.SpanKindInternal,
		}, attrs...)
		queue.SetSpanCorrelationAttributes(ctx, span)
		c.SetContext(ctx)
		defer span.End()

		err := c.Next()
		if err != nil {
			span.RecordError(err)
			span.SetStatus(tracing.StatusError, err.Error())
		}
		return err
	})
}

func RateLimit(limiter queue.RateLimiter, keyFn RateLimitKeyFunc) queue.Middleware {
	return func(next queue.HandlerFunc) queue.HandlerFunc {
		return func(ctx context.Context, msg *queue.Message) error {
			if limiter == nil || keyFn == nil {
				return next(ctx, msg)
			}
			if ctx == nil {
				ctx = context.Background()
			}
			key, limit, window, apply, err := keyFn(ctx, msg)
			if err != nil {
				return err
			}
			if !apply {
				return next(ctx, msg)
			}
			allowed, retryIn, err := limiter.Allow(ctx, key, limit, window)
			if err != nil {
				return err
			}
			if !allowed {
				return queue.RateLimited(retryIn, queue.ErrRateLimited)
			}
			return next(ctx, msg)
		}
	}
}

func StaticRateLimitKey(key string, limit int, window time.Duration) RateLimitKeyFunc {
	return func(context.Context, *queue.Message) (string, int, time.Duration, bool, error) {
		return key, limit, window, true, nil
	}
}
