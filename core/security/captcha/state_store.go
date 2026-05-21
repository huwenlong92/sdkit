package captcha

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/security/state"
)

type StateStore struct {
	store state.Store
	ttl   time.Duration
}

func NewStateStore(store state.Store, ttl time.Duration) *StateStore {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &StateStore{store: store, ttl: ttl}
}

func (s *StateStore) Set(id string, value string) error {
	if s == nil || s.store == nil {
		return ErrInvalidToken
	}
	return s.store.Set(context.Background(), id, value, s.ttl)
}

func (s *StateStore) Get(id string, clear bool) string {
	if s == nil || s.store == nil {
		return ""
	}
	value, ok, err := s.store.Get(context.Background(), id)
	if err != nil || !ok {
		return ""
	}
	if clear {
		_ = s.store.Delete(context.Background(), id)
	}
	return value
}

func (s *StateStore) Verify(id string, answer string, clear bool) bool {
	return s.Get(id, clear) == answer
}
