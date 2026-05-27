package middleware

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"
)

type RateLimitKeyFunc func(ctx context.Context, msg *queue.Message) (key string, limit int, window time.Duration, apply bool, err error)

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
