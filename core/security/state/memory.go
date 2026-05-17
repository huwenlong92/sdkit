package state

import (
	"context"
	"strconv"
	"sync"
	"time"
)

type MemoryStore struct {
	mu    sync.Mutex
	items map[string]memoryItem
}

type memoryItem struct {
	value     string
	expiresAt time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: make(map[string]memoryItem)}
}

func (s *MemoryStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	item, ok := s.liveLocked(key, now)
	var n int64
	if ok {
		n, _ = strconv.ParseInt(item.value, 10, 64)
	}
	n++
	s.items[key] = memoryItem{value: strconv.FormatInt(n, 10), expiresAt: now.Add(ttl)}
	return n, nil
}

func (s *MemoryStore) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = memoryItem{value: value, expiresAt: time.Now().Add(ttl)}
	return nil
}

func (s *MemoryStore) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if _, ok := s.liveLocked(key, now); ok {
		return false, nil
	}
	s.items[key] = memoryItem{value: value, expiresAt: now.Add(ttl)}
	return true, nil
}

func (s *MemoryStore) Get(ctx context.Context, key string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.liveLocked(key, time.Now())
	if !ok {
		return "", false, nil
	}
	return item.value, true, nil
}

func (s *MemoryStore) Exists(ctx context.Context, key string) (bool, error) {
	_, ok, err := s.Get(ctx, key)
	return ok, err
}

func (s *MemoryStore) TTL(ctx context.Context, key string) (time.Duration, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.liveLocked(key, time.Now())
	if !ok {
		return 0, false, nil
	}
	return time.Until(item.expiresAt), true, nil
}

func (s *MemoryStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
	return nil
}

func (s *MemoryStore) liveLocked(key string, now time.Time) (memoryItem, bool) {
	item, ok := s.items[key]
	if !ok {
		return memoryItem{}, false
	}
	if !item.expiresAt.IsZero() && now.After(item.expiresAt) {
		delete(s.items, key)
		return memoryItem{}, false
	}
	return item, true
}
