package strategy

import (
	"context"

	"github.com/huwenlong92/sdkit/pkg/ratelimit/store"
)

// TokenBucket 令牌桶限流，每 key 独立计数
type TokenBucket struct {
	rate  float64
	burst int
	store store.Store
}

// NewTokenBucket 创建令牌桶，store 为 nil 时默认使用内存存储
func NewTokenBucket(rate float64, burst int, stores ...store.Store) *TokenBucket {
	s := pickStore(stores)
	return &TokenBucket{rate: rate, burst: burst, store: s}
}

func (t *TokenBucket) Allow(key string) bool { return t.AllowN(key, 1) }

func (t *TokenBucket) AllowN(key string, n int) bool {
	return t.AllowNContext(context.Background(), key, n)
}

func (t *TokenBucket) AllowContext(ctx context.Context, key string) bool {
	return t.AllowNContext(ctx, key, 1)
}

func (t *TokenBucket) AllowNContext(ctx context.Context, key string, n int) bool {
	for i := 0; i < n; i++ {
		ok, _ := takeToken(ctx, t.store, key, t.rate, t.burst)
		if !ok {
			return false
		}
	}
	return true
}

func takeToken(ctx context.Context, s store.Store, key string, rate float64, burst int) (bool, error) {
	if ctxStore, ok := s.(store.ContextStore); ok {
		return ctxStore.TakeTokenContext(ctx, key, rate, burst)
	}
	return s.TakeToken(key, rate, burst)
}
