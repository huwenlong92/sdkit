package store

import (
	"context"
	"strings"
	"time"
)

type PrefixedStore struct {
	store  Store
	prefix string
}

func NewPrefixedStore(store Store, prefix string) *PrefixedStore {
	return &PrefixedStore{store: store, prefix: prefix}
}

func (s *PrefixedStore) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return s.store.Set(ctx, s.key(key), value, ttl)
}

func (s *PrefixedStore) Get(ctx context.Context, key string) (string, bool, error) {
	return s.store.Get(ctx, s.key(key))
}

func (s *PrefixedStore) Delete(ctx context.Context, key string) error {
	return s.store.Delete(ctx, s.key(key))
}

func (s *PrefixedStore) key(key string) string {
	return s.prefix + strings.TrimSpace(key)
}
