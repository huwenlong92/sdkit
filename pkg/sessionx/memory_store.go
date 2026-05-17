package sessionx

import (
	"context"
	"sync"
	"time"
)

type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	prefix   string
}

func NewMemoryStore() *MemoryStore {
	return NewMemoryStoreWithPrefix("session:")
}

func NewMemoryStoreWithPrefix(prefix string) *MemoryStore {
	m := &MemoryStore{sessions: make(map[string]*Session), prefix: prefix}
	go m.gc()
	return m
}

func (m *MemoryStore) key(id string) string {
	return m.prefix + id
}

func (m *MemoryStore) Get(_ context.Context, id string) (*Session, bool) {
	m.mu.RLock()
	s, ok := m.sessions[m.key(id)]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(s.ExpiresAt) {
		m.mu.Lock()
		delete(m.sessions, m.key(id))
		m.mu.Unlock()
		return nil, false
	}
	return s, true
}

func (m *MemoryStore) Set(_ context.Context, s *Session, ttl time.Duration) error {
	s.ExpiresAt = time.Now().Add(ttl)
	m.mu.Lock()
	m.sessions[m.key(s.ID)] = s
	m.mu.Unlock()
	return nil
}

func (m *MemoryStore) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	delete(m.sessions, m.key(id))
	m.mu.Unlock()
	return nil
}

func (m *MemoryStore) gc() {
	for {
		time.Sleep(time.Minute)
		now := time.Now()
		m.mu.Lock()
		for k, s := range m.sessions {
			if now.After(s.ExpiresAt) {
				delete(m.sessions, k)
			}
		}
		m.mu.Unlock()
	}
}
