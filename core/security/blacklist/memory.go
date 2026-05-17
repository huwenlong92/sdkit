package blacklist

import (
	"context"
	"sync"
	"time"
)

type MemoryStore struct {
	mu      sync.Mutex
	entries map[string]Entry
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{entries: make(map[string]Entry)}
}

func (s *MemoryStore) Add(ctx context.Context, entry Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key(entry.Type, entry.Value)] = entry
	return nil
}

func (s *MemoryStore) Contains(ctx context.Context, typ string, value string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[key(typ, value)]
	if !ok {
		return false, nil
	}
	if entry.ExpiredAt > 0 && entry.ExpiredAt <= time.Now().Unix() {
		delete(s.entries, key(typ, value))
		return false, nil
	}
	return true, nil
}

func (s *MemoryStore) Remove(ctx context.Context, typ string, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key(typ, value))
	return nil
}

func key(typ, value string) string {
	return typ + ":" + value
}
