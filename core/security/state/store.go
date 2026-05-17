package state

import (
	"context"
	"time"
)

type Store interface {
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error)
	Get(ctx context.Context, key string) (string, bool, error)
	Exists(ctx context.Context, key string) (bool, error)
	TTL(ctx context.Context, key string) (time.Duration, bool, error)
	Delete(ctx context.Context, key string) error
}
