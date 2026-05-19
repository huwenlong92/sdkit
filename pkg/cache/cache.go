package cache

import (
	"context"
	"time"
)

type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, keys ...string) (int64, error)
	Incr(ctx context.Context, key string) (int64, error)
	TTL(ctx context.Context, key string) (time.Duration, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	Gets(ctx context.Context, keys []string) (map[string]string, []string)
	Sets(ctx context.Context, values map[string]string, ttl time.Duration) error
	Delete(ctx context.Context, keys []string) error
	Close() error
}
