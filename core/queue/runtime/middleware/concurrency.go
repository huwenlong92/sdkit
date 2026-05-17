package middleware

import (
	"context"

	"github.com/huwenlong92/sdkit/core/queue"
)

type ConcurrencyKeyFunc func(ctx context.Context, msg *queue.Message) (key string, apply bool, err error)

func Concurrency(limiter queue.ConcurrencyLimiter, keyFns ...ConcurrencyKeyFunc) queue.Middleware {
	if limiter == nil {
		return nil
	}
	keyFn := firstConcurrencyKeyFunc(keyFns...)
	return func(next queue.HandlerFunc) queue.HandlerFunc {
		return func(ctx context.Context, msg *queue.Message) error {
			if ctx == nil {
				ctx = context.Background()
			}
			key, apply, err := keyFn(ctx, msg)
			if err != nil {
				return err
			}
			if !apply {
				return next(ctx, msg)
			}
			if err := limiter.Acquire(ctx, key); err != nil {
				return err
			}
			defer limiter.Release(context.WithoutCancel(ctx), key)
			return next(ctx, msg)
		}
	}
}

func StaticConcurrencyKey(key string) ConcurrencyKeyFunc {
	return func(context.Context, *queue.Message) (string, bool, error) {
		return key, key != "", nil
	}
}

func firstConcurrencyKeyFunc(keyFns ...ConcurrencyKeyFunc) ConcurrencyKeyFunc {
	for _, keyFn := range keyFns {
		if keyFn != nil {
			return keyFn
		}
	}
	return messageConcurrencyKey
}

func messageConcurrencyKey(_ context.Context, msg *queue.Message) (string, bool, error) {
	if key, ok := queue.MessageMetadataString(msg, queue.MessageMetadataConcurrencyKey); ok {
		return key, true, nil
	}
	if msg != nil && msg.Type != "" {
		return msg.Type, true, nil
	}
	return "queue", true, nil
}
