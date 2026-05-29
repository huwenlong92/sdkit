package captcha

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/security/captcha/store"
)

type Base64Store struct {
	store store.Store
	ttl   time.Duration
}

func NewBase64Store(store store.Store, ttl time.Duration) *Base64Store {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Base64Store{store: store, ttl: ttl}
}

func (s *Base64Store) Set(id string, value string) error {
	if s == nil || s.store == nil {
		return ErrInvalidToken
	}
	return s.store.Set(context.Background(), id, value, s.ttl)
}

func (s *Base64Store) Get(id string, clear bool) string {
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

func (s *Base64Store) Verify(id string, answer string, clear bool) bool {
	return s.Get(id, clear) == answer
}
