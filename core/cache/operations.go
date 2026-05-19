package cache

import (
	"context"
	"time"
)

// Get reads a string value from the default cache.
func Get(ctx context.Context, key string) (string, error) {
	return GetWith(ctx, nil, key)
}

// GetWith reads a string value from the provided cache.
func GetWith(ctx context.Context, c Cache, key string) (string, error) {
	if c == nil {
		c = Default()
	}
	return c.Get(ctx, key)
}

// Set writes a string value to the default cache.
func Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return SetWith(ctx, nil, key, value, ttl)
}

// SetWith writes a string value to the provided cache.
func SetWith(ctx context.Context, c Cache, key, value string, ttl time.Duration) error {
	if c == nil {
		c = Default()
	}
	return c.Set(ctx, key, value, ttl)
}

// Del removes keys from the default cache.
func Del(ctx context.Context, keys ...string) error {
	return DelWith(ctx, nil, keys...)
}

// DelWith removes keys from the provided cache.
func DelWith(ctx context.Context, c Cache, keys ...string) error {
	if c == nil {
		c = Default()
	}
	return c.Del(ctx, keys...)
}

// Exists reports how many keys exist in the default cache.
func Exists(ctx context.Context, keys ...string) (int64, error) {
	return ExistsWith(ctx, nil, keys...)
}

// ExistsWith reports how many keys exist in the provided cache.
func ExistsWith(ctx context.Context, c Cache, keys ...string) (int64, error) {
	if c == nil {
		c = Default()
	}
	return c.Exists(ctx, keys...)
}

// Incr increments a key in the default cache.
func Incr(ctx context.Context, key string) (int64, error) {
	return IncrWith(ctx, nil, key)
}

// IncrWith increments a key in the provided cache.
func IncrWith(ctx context.Context, c Cache, key string) (int64, error) {
	if c == nil {
		c = Default()
	}
	return c.Incr(ctx, key)
}

// TTL returns the ttl for a key in the default cache.
func TTL(ctx context.Context, key string) (time.Duration, error) {
	return TTLWith(ctx, nil, key)
}

// TTLWith returns the ttl for a key in the provided cache.
func TTLWith(ctx context.Context, c Cache, key string) (time.Duration, error) {
	if c == nil {
		c = Default()
	}
	return c.TTL(ctx, key)
}

// Expire updates the expiration for a key in the default cache.
func Expire(ctx context.Context, key string, ttl time.Duration) error {
	return ExpireWith(ctx, nil, key, ttl)
}

// ExpireWith updates the expiration for a key in the provided cache.
func ExpireWith(ctx context.Context, c Cache, key string, ttl time.Duration) error {
	if c == nil {
		c = Default()
	}
	return c.Expire(ctx, key, ttl)
}

// Gets reads multiple string values from the default cache.
func Gets(ctx context.Context, keys []string) (map[string]string, []string) {
	return GetsWith(ctx, nil, keys)
}

// GetsWith reads multiple string values from the provided cache.
func GetsWith(ctx context.Context, c Cache, keys []string) (map[string]string, []string) {
	if c == nil {
		c = Default()
	}
	return c.Gets(ctx, keys)
}

// Sets writes multiple string values to the default cache.
func Sets(ctx context.Context, values map[string]string, ttl time.Duration) error {
	return SetsWith(ctx, nil, values, ttl)
}

// SetsWith writes multiple string values to the provided cache.
func SetsWith(ctx context.Context, c Cache, values map[string]string, ttl time.Duration) error {
	if c == nil {
		c = Default()
	}
	return c.Sets(ctx, values, ttl)
}

// Delete removes keys from the default cache.
func Delete(ctx context.Context, keys []string) error {
	return DeleteWith(ctx, nil, keys)
}

// DeleteWith removes keys from the provided cache.
func DeleteWith(ctx context.Context, c Cache, keys []string) error {
	if c == nil {
		c = Default()
	}
	return c.Delete(ctx, keys)
}
