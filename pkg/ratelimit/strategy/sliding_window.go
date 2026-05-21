package strategy

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/pkg/ratelimit/store"
)

// SlidingWindow 滑动窗口限流，按时间戳精确计数
type SlidingWindow struct {
	window time.Duration
	limit  int
	store  store.Store
}

// NewSlidingWindow 创建滑动窗口，store 为 nil 时默认使用内存存储
func NewSlidingWindow(window time.Duration, limit int, stores ...store.Store) *SlidingWindow {
	s := pickStore(stores)
	return &SlidingWindow{window: window, limit: limit, store: s}
}

func (sw *SlidingWindow) Allow(key string) bool { return sw.AllowN(key, 1) }

func (sw *SlidingWindow) AllowN(key string, n int) bool {
	return sw.AllowNContext(context.Background(), key, n)
}

func (sw *SlidingWindow) AllowContext(ctx context.Context, key string) bool {
	return sw.AllowNContext(ctx, key, 1)
}

func (sw *SlidingWindow) AllowNContext(ctx context.Context, key string, n int) bool {
	count, _ := windowAdd(ctx, sw.store, key, time.Now().UnixNano(), n, sw.window)
	return count <= sw.limit
}

func windowAdd(ctx context.Context, s store.Store, key string, ts int64, n int, window time.Duration) (int, error) {
	if ctxStore, ok := s.(store.ContextStore); ok {
		return ctxStore.WindowAddContext(ctx, key, ts, n, window)
	}
	return s.WindowAdd(key, ts, n, window)
}
