package middleware

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type RateLimitKeyFunc func(ctx context.Context, msg *queue.Message) (key string, limit int, window time.Duration, apply bool, err error)

func Tracing() queue.Middleware {
	return queue.ContextChain(func(c *queue.HandlerContext) error {
		msg := c.Message
		name := "handler::task"
		attrs := []attribute.KeyValue{
			attribute.String("worker.component", "queue"),
		}
		if msg != nil {
			name = "handler::" + msg.Type
			attrs = append(attrs,
				attribute.String("messaging.destination.name", msg.Queue),
				attribute.String("messaging.message.id", msg.ID),
				attribute.String("messaging.message.type", msg.Type),
				attribute.Int("messaging.message.retry_count", msg.RetryCount),
				attribute.Int("messaging.message.max_retry", msg.MaxRetry),
			)
		}

		ctx, span := otel.Tracer("sdkitgo/core/queue").Start(c.Context(), name,
			oteltrace.WithSpanKind(oteltrace.SpanKindInternal),
			oteltrace.WithAttributes(attrs...),
		)
		queue.SetSpanCorrelationAttributes(ctx, span)
		c.SetContext(ctx)
		defer span.End()

		err := c.Next()
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
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
