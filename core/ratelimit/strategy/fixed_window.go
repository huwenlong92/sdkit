package strategy

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/ratelimit/store"
)

// FixedWindow 固定窗口计数器，每 key 独立
type FixedWindow struct {
	window time.Duration
	limit  int
	store  store.Store
}

// NewFixedWindow 创建固定窗口，store 为 nil 时默认使用内存存储
func NewFixedWindow(window time.Duration, limit int, stores ...store.Store) *FixedWindow {
	s := pickStore(stores)
	return &FixedWindow{window: window, limit: limit, store: s}
}

func (w *FixedWindow) Allow(key string) bool { return w.AllowN(key, 1) }

func (w *FixedWindow) AllowN(key string, n int) bool {
	return w.AllowNContext(context.Background(), key, n)
}

func (w *FixedWindow) AllowContext(ctx context.Context, key string) bool {
	return w.AllowNContext(ctx, key, 1)
}

func (w *FixedWindow) AllowNContext(ctx context.Context, key string, n int) bool {
	count, _ := counter(ctx, w.store, key, n, w.window)
	return count <= w.limit
}

func counter(ctx context.Context, s store.Store, key string, n int, ttl time.Duration) (int, error) {
	if ctxStore, ok := s.(store.ContextStore); ok {
		return ctxStore.CounterContext(ctx, key, n, ttl)
	}
	return s.Counter(key, n, ttl)
}

// pickStore 选取存储，nil 时创建独立内存实例（测试隔离、生产可共享 Redis）
func pickStore(stores []store.Store) store.Store {
	if len(stores) > 0 && stores[0] != nil {
		return stores[0]
	}
	return store.NewMemoryStore()
}
