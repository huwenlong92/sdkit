package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/queue"

	"go.uber.org/zap"
)

const defaultUnlockTimeout = 5 * time.Second

type LockKeyFunc func(ctx context.Context, msg *queue.Message) (key string, ttl time.Duration, apply bool, err error)

func Lock(locker queue.Locker, keyFns ...LockKeyFunc) queue.Middleware {
	keyFn := firstLockKeyFunc(keyFns...)
	return func(next queue.HandlerFunc) queue.HandlerFunc {
		return func(ctx context.Context, msg *queue.Message) (err error) {
			if locker == nil || keyFn == nil {
				return next(ctx, msg)
			}
			if ctx == nil {
				ctx = context.Background()
			}
			key, ttl, apply, err := keyFn(ctx, msg)
			if err != nil {
				return err
			}
			if !apply {
				return next(ctx, msg)
			}
			if key == "" {
				return fmt.Errorf("queue lock key is required")
			}
			unlock, ok, err := locker.Lock(ctx, key, ttl)
			if err != nil {
				return err
			}
			if !ok {
				return queue.ErrLockNotAcquired
			}
			defer func() {
				if unlock == nil {
					return
				}
				unlockCtx, cancel := context.WithTimeout(context.Background(), defaultUnlockTimeout)
				defer cancel()
				if unlockErr := unlock(unlockCtx); unlockErr != nil {
					queueLogger(ctx, logger.L).Error("队列任务释放锁失败",
						append(messageFields(msg),
							zap.String("lock_key", key),
							zap.Error(unlockErr),
						)...,
					)
				}
			}()
			return next(ctx, msg)
		}
	}
}

func StaticLockKey(key string, ttl time.Duration) LockKeyFunc {
	return func(context.Context, *queue.Message) (string, time.Duration, bool, error) {
		return key, ttl, key != "", nil
	}
}

func firstLockKeyFunc(keyFns ...LockKeyFunc) LockKeyFunc {
	for _, keyFn := range keyFns {
		if keyFn != nil {
			return keyFn
		}
	}
	return messageLockKey
}

func messageLockKey(_ context.Context, msg *queue.Message) (string, time.Duration, bool, error) {
	value, ok := queue.MessageMetadataValue(msg, queue.MessageMetadataLockKey)
	if !ok {
		return "", 0, false, nil
	}
	key, ok := value.(string)
	if !ok || key == "" {
		return "", 0, false, nil
	}
	ttl, ok := queue.MessageMetadataDuration(msg, queue.MessageMetadataLockTTL)
	if !ok || ttl <= 0 {
		return "", 0, false, fmt.Errorf("queue lock ttl is required")
	}
	return key, ttl, true, nil
}
