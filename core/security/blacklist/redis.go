package blacklist

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/security/state"
)

type RedisLikeStore struct {
	state state.Store
}

func NewRedisLikeStore(store state.Store) *RedisLikeStore {
	return &RedisLikeStore{state: store}
}

func (s *RedisLikeStore) Add(ctx context.Context, entry Entry) error {
	ttl := time.Hour * 24 * 365 * 20
	if entry.ExpiredAt > 0 {
		ttl = time.Until(time.Unix(entry.ExpiredAt, 0))
		if ttl <= 0 {
			return nil
		}
	}
	return s.state.Set(ctx, "security:blacklist:"+key(entry.Type, entry.Value), entry.Reason, ttl)
}

func (s *RedisLikeStore) Contains(ctx context.Context, typ string, value string) (bool, error) {
	return s.state.Exists(ctx, "security:blacklist:"+key(typ, value))
}

func (s *RedisLikeStore) Remove(ctx context.Context, typ string, value string) error {
	return s.state.Delete(ctx, "security:blacklist:"+key(typ, value))
}
