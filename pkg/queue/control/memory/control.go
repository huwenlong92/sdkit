package memorycontrol

import (
	"context"
	"fmt"
	"sync"
	"time"

	corequeue "github.com/huwenlong92/sdkit/core/queue"
)

type Locker struct {
	mu    sync.Mutex
	locks map[string]memoryLock
}

type memoryLock struct {
	token     string
	expiresAt time.Time
}

func NewLocker() *Locker {
	return &Locker{locks: map[string]memoryLock{}}
}

func (l *Locker) Lock(ctx context.Context, key string, ttl time.Duration) (func(context.Context) error, bool, error) {
	if l == nil {
		return nil, false, corequeue.ErrNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if ttl <= 0 {
		return nil, false, fmt.Errorf("queue lock ttl must be positive")
	}
	now := time.Now()
	token := fmt.Sprintf("%d", now.UnixNano())
	l.mu.Lock()
	if current, ok := l.locks[key]; ok && now.Before(current.expiresAt) {
		l.mu.Unlock()
		return nil, false, corequeue.ErrLockNotAcquired
	}
	l.locks[key] = memoryLock{token: token, expiresAt: now.Add(ttl)}
	l.mu.Unlock()
	unlock := func(unlockCtx context.Context) error {
		if err := unlockCtx.Err(); err != nil {
			return err
		}
		l.mu.Lock()
		defer l.mu.Unlock()
		current, ok := l.locks[key]
		if !ok || current.token != token {
			return nil
		}
		delete(l.locks, key)
		return nil
	}
	return unlock, true, nil
}

type Idempotency struct {
	mu   sync.Mutex
	done map[string]time.Time
}

func NewIdempotency() *Idempotency {
	return &Idempotency{done: map[string]time.Time{}}
}

func (i *Idempotency) Done(ctx context.Context, key string) (bool, error) {
	if i == nil {
		return false, corequeue.ErrNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	expiresAt, ok := i.done[key]
	if !ok {
		return false, nil
	}
	if time.Now().After(expiresAt) {
		delete(i.done, key)
		return false, nil
	}
	return true, nil
}

func (i *Idempotency) MarkDone(ctx context.Context, key string, ttl time.Duration) error {
	if i == nil {
		return corequeue.ErrNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if ttl <= 0 {
		return fmt.Errorf("queue idempotency ttl must be positive")
	}
	i.mu.Lock()
	i.done[key] = time.Now().Add(ttl)
	i.mu.Unlock()
	return nil
}

type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]rateBucket
}

type rateBucket struct {
	count     int
	expiresAt time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{buckets: map[string]rateBucket{}}
}

func (l *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	if l == nil {
		return false, 0, corequeue.ErrNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return false, 0, err
	}
	if limit <= 0 || window <= 0 {
		return false, 0, fmt.Errorf("queue rate limit and window must be positive")
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket := l.buckets[key]
	if bucket.expiresAt.IsZero() || now.After(bucket.expiresAt) {
		bucket = rateBucket{expiresAt: now.Add(window)}
	}
	bucket.count++
	l.buckets[key] = bucket
	if bucket.count <= limit {
		return true, 0, nil
	}
	retryIn := time.Until(bucket.expiresAt)
	if retryIn < 0 {
		retryIn = 0
	}
	return false, retryIn, corequeue.RateLimited(retryIn, corequeue.ErrRateLimited)
}
