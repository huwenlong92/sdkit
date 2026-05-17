package store

import (
	"context"
	"sync"
	"time"

	xrate "golang.org/x/time/rate"
)

// MemoryStore 内存存储实现
type MemoryStore struct {
	mu       sync.RWMutex
	counters map[string]*counterSlot
	windows  map[string][]int64
	tokens   map[string]*xrate.Limiter
	stopCh   chan struct{}
}

type counterSlot struct {
	count  int
	window int64
}

// NewMemoryStore 创建内存存储
func NewMemoryStore() *MemoryStore {
	s := &MemoryStore{
		counters: make(map[string]*counterSlot),
		windows:  make(map[string][]int64),
		tokens:   make(map[string]*xrate.Limiter),
		stopCh:   make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

// ======================== Counter ========================

func (s *MemoryStore) Counter(key string, n int, ttl time.Duration) (int, error) {
	return s.CounterContext(context.Background(), key, n, ttl)
}

func (s *MemoryStore) CounterContext(_ context.Context, key string, n int, ttl time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur := time.Now().UnixNano() / int64(ttl)
	slot, ok := s.counters[key]
	if !ok || slot.window != cur {
		s.counters[key] = &counterSlot{count: n, window: cur}
		return n, nil
	}
	slot.count += n
	return slot.count, nil
}

// ======================== WindowAdd ========================

func (s *MemoryStore) WindowAdd(key string, ts int64, n int, window time.Duration) (int, error) {
	return s.WindowAddContext(context.Background(), key, ts, n, window)
}

func (s *MemoryStore) WindowAddContext(_ context.Context, key string, ts int64, n int, window time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := ts - int64(window)
	slots := s.windows[key]

	// 裁剪过期
	start := 0
	for i, t := range slots {
		if t > cutoff {
			start = i
			break
		}
		start = i + 1
	}
	slots = slots[start:]

	for i := 0; i < n; i++ {
		slots = append(slots, ts)
	}
	s.windows[key] = slots
	return len(slots), nil
}

// ======================== TakeToken ========================

func (s *MemoryStore) TakeToken(key string, rate float64, burst int) (bool, error) {
	return s.TakeTokenContext(context.Background(), key, rate, burst)
}

func (s *MemoryStore) TakeTokenContext(_ context.Context, key string, rate float64, burst int) (bool, error) {
	s.mu.RLock()
	l, ok := s.tokens[key]
	s.mu.RUnlock()

	if !ok {
		l = xrate.NewLimiter(xrate.Limit(rate), burst)
		s.mu.Lock()
		s.tokens[key] = l
		s.mu.Unlock()
	}
	return l.AllowN(time.Now(), 1), nil
}

// ======================== Cleanup ========================

func (s *MemoryStore) Cleanup() {
	s.cleanup()
}

func (s *MemoryStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixNano()
	// 清理 counters（保留最近 2 个窗口）
	for k, slot := range s.counters {
		if slot.window < now/int64(time.Second)-10 {
			delete(s.counters, k)
		}
	}
	// 清理 tokens（恢复满的不需要保留）
	for k, l := range s.tokens {
		if l.Tokens() >= float64(l.Burst()) {
			delete(s.tokens, k)
		}
	}
	// 清理 windows（保留最近一小时）
	cutoff := now - int64(time.Hour)
	for k, slots := range s.windows {
		if len(slots) == 0 {
			delete(s.windows, k)
			continue
		}
		if slots[len(slots)-1] < cutoff {
			delete(s.windows, k)
		}
	}
}

func (s *MemoryStore) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopCh:
			return
		}
	}
}

// Close 停止清理协程
func (s *MemoryStore) Close() error {
	close(s.stopCh)
	return nil
}
