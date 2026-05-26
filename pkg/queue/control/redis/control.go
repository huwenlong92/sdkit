package rediscontrol

import (
	"context"
	"fmt"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"

	"github.com/redis/go-redis/v9"
)

type Locker struct {
	client redis.Cmdable
	prefix string
}

func NewLocker(client redis.Cmdable, prefix string) *Locker {
	if prefix == "" {
		prefix = "queue:lock:"
	}
	return &Locker{client: client, prefix: prefix}
}

func (l *Locker) Lock(ctx context.Context, key string, ttl time.Duration) (func(context.Context) error, bool, error) {
	if l == nil || l.client == nil {
		return nil, false, queue.ErrNotInitialized
	}
	if ttl <= 0 {
		return nil, false, fmt.Errorf("queue lock ttl must be positive")
	}
	token := fmt.Sprintf("%d", time.Now().UnixNano())
	fullKey := l.prefix + key
	ok, err := l.client.SetNX(ctx, fullKey, token, ttl).Result()
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, queue.ErrLockNotAcquired
	}
	unlock := func(unlockCtx context.Context) error {
		const script = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0`
		return l.client.Eval(unlockCtx, script, []string{fullKey}, token).Err()
	}
	return unlock, true, nil
}

type Idempotency struct {
	client redis.Cmdable
	prefix string
}

func NewIdempotency(client redis.Cmdable, prefix string) *Idempotency {
	if prefix == "" {
		prefix = "queue:done:"
	}
	return &Idempotency{client: client, prefix: prefix}
}

func (i *Idempotency) Done(ctx context.Context, key string) (bool, error) {
	if i == nil || i.client == nil {
		return false, queue.ErrNotInitialized
	}
	n, err := i.client.Exists(ctx, i.prefix+key).Result()
	return n > 0, err
}

func (i *Idempotency) MarkDone(ctx context.Context, key string, ttl time.Duration) error {
	if i == nil || i.client == nil {
		return queue.ErrNotInitialized
	}
	if ttl <= 0 {
		return fmt.Errorf("queue idempotency ttl must be positive")
	}
	return i.client.Set(ctx, i.prefix+key, "1", ttl).Err()
}

type RateLimiter struct {
	client redis.Cmdable
	prefix string
}

func NewRateLimiter(client redis.Cmdable, prefix string) *RateLimiter {
	if prefix == "" {
		prefix = "queue:rate:"
	}
	return &RateLimiter{client: client, prefix: prefix}
}

func (l *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	if l == nil || l.client == nil {
		return false, 0, queue.ErrNotInitialized
	}
	if limit <= 0 || window <= 0 {
		return false, 0, fmt.Errorf("queue rate limit and window must be positive")
	}
	fullKey := l.prefix + key
	count, err := l.client.Incr(ctx, fullKey).Result()
	if err != nil {
		return false, 0, err
	}
	if count == 1 {
		if err := l.client.Expire(ctx, fullKey, window).Err(); err != nil {
			return false, 0, err
		}
	}
	if count <= int64(limit) {
		return true, 0, nil
	}
	ttl, err := l.client.TTL(ctx, fullKey).Result()
	if err != nil {
		return false, 0, err
	}
	if ttl < 0 {
		ttl = window
	}
	return false, ttl, queue.RateLimited(ttl, queue.ErrRateLimited)
}
