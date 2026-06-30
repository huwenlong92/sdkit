package reporter

import (
	"context"
	"sync"
	"time"
)

type MemoryProgressStore struct {
	mu    sync.Mutex
	items map[string]memoryProgressItem
}

type memoryProgressItem struct {
	snapshot  ProgressSnapshot
	expiresAt time.Time
}

func NewMemoryProgressStore() *MemoryProgressStore {
	return &MemoryProgressStore{items: make(map[string]memoryProgressItem)}
}

func (s *MemoryProgressStore) Save(ctx context.Context, key string, snapshot ProgressSnapshot, ttl time.Duration) error {
	if s == nil || key == "" {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	expiresAt := time.Time{}
	if ttl > 0 {
		expiresAt = now().Add(ttl)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = make(map[string]memoryProgressItem)
	}
	s.items[key] = memoryProgressItem{snapshot: cloneProgressSnapshot(snapshot), expiresAt: expiresAt}
	return nil
}

func (s *MemoryProgressStore) Load(ctx context.Context, key string) (ProgressSnapshot, bool, error) {
	if s == nil || key == "" {
		return ProgressSnapshot{}, false, nil
	}
	select {
	case <-ctx.Done():
		return ProgressSnapshot{}, false, ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[key]
	if !ok {
		return ProgressSnapshot{}, false, nil
	}
	if !item.expiresAt.IsZero() && now().After(item.expiresAt) {
		delete(s.items, key)
		return ProgressSnapshot{}, false, nil
	}
	return cloneProgressSnapshot(item.snapshot), true, nil
}

func (s *MemoryProgressStore) Delete(ctx context.Context, key string) error {
	if s == nil || key == "" {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
	return nil
}

func cloneProgressSnapshot(snapshot ProgressSnapshot) ProgressSnapshot {
	snapshot.Subject = cloneMap(snapshot.Subject)
	if len(snapshot.Steps) > 0 {
		steps := make([]ProgressStep, len(snapshot.Steps))
		for i, step := range snapshot.Steps {
			step.Detail = cloneMap(step.Detail)
			steps[i] = step
		}
		snapshot.Steps = steps
	}
	if len(snapshot.Recent) > 0 {
		recent := make([]Event, len(snapshot.Recent))
		for i, event := range snapshot.Recent {
			recent[i] = event.Clone()
		}
		snapshot.Recent = recent
	}
	return snapshot
}
