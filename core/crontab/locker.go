package crontab

import (
	"context"
	"time"
)

type Lock interface {
	Release(ctx context.Context) error
	Unlock(ctx context.Context) error
	Refresh(ctx context.Context, ttl time.Duration) error
}

type Locker interface {
	Acquire(ctx context.Context, key string, ttl time.Duration) (Lock, bool, error)
	TryLock(ctx context.Context, key string, ttl time.Duration) (Lock, bool, error)
}

type NoopLocker struct{}

func (NoopLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (Lock, bool, error) {
	return noopLock{}, true, nil
}

func (NoopLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (Lock, bool, error) {
	return noopLock{}, true, nil
}

type noopLock struct{}

func (noopLock) Release(ctx context.Context) error { return nil }
func (noopLock) Unlock(ctx context.Context) error  { return nil }
func (noopLock) Refresh(ctx context.Context, ttl time.Duration) error {
	return nil
}
