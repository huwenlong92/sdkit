package store

import (
	"context"
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

func (s *MemoryStore) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = memoryItem{value: value, expiresAt: time.Now().Add(ttl)}
	return nil
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
