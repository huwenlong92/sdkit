package redis

import (
	"context"
	"errors"
	"time"

	pkgcache "github.com/huwenlong92/sdkit/pkg/cache"

	goredis "github.com/redis/go-redis/v9"
)

var _ pkgcache.Cache = (*Store)(nil)

type Store struct {
	Client *goredis.Client
	Prefix string
}

func New(client *goredis.Client, prefix string) *Store {
	if prefix == "" {
		prefix = "cache:"
	}
	return &Store{Client: client, Prefix: prefix}
}

func (r *Store) key(k string) string { return r.Prefix + k }

func (r *Store) Get(ctx context.Context, key string) (string, error) {
	val, err := r.Client.Get(ctx, r.key(key)).Result()
	if errors.Is(err, goredis.Nil) {
		return "", nil
	}
	return val, err
}

func (r *Store) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return r.Client.Set(ctx, r.key(key), value, ttl).Err()
}

func (r *Store) Del(ctx context.Context, keys ...string) error {
	for i, k := range keys {
		keys[i] = r.key(k)
	}
	return r.Client.Del(ctx, keys...).Err()
}

func (r *Store) Exists(ctx context.Context, keys ...string) (int64, error) {
	for i, k := range keys {
		keys[i] = r.key(k)
	}
	return r.Client.Exists(ctx, keys...).Result()
}

func (r *Store) Incr(ctx context.Context, key string) (int64, error) {
	return r.Client.Incr(ctx, r.key(key)).Result()
}

func (r *Store) Gets(ctx context.Context, keys []string) (map[string]string, []string) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}

	prefixed := make([]string, len(keys))
	for i, k := range keys {
		prefixed[i] = r.key(k)
	}

	vals, err := r.Client.MGet(ctx, prefixed...).Result()
	if err != nil {
		return nil, nil
	}

	result := make(map[string]string, len(keys))
	var missing []string
	for i, v := range vals {
		if v == nil {
			missing = append(missing, keys[i])
		} else {
			result[keys[i]] = v.(string)
		}
	}
	return result, missing
}

func (r *Store) Sets(ctx context.Context, values map[string]string, ttl time.Duration) error {
	if len(values) == 0 {
		return nil
	}

	pipe := r.Client.Pipeline()
	for k, v := range values {
		pipe.Set(ctx, r.key(k), v, ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (r *Store) Delete(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	prefixed := make([]string, len(keys))
	for i, k := range keys {
		prefixed[i] = r.key(k)
	}
	return r.Client.Del(ctx, prefixed...).Err()
}

func (r *Store) TTL(ctx context.Context, key string) (time.Duration, error) {
	return r.Client.TTL(ctx, r.key(key)).Result()
}

func (r *Store) Close() error {
	return nil
}
