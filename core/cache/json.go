package cache

import (
	"context"
	"errors"
	"time"

	"github.com/huwenlong92/sdkit/core/jsonx"
)

// Set stores value as JSON through the cache abstraction.
func Set(ctx context.Context, c Cache, key string, value any, ttl time.Duration) error {
	if c == nil {
		c = Default()
	}
	b, err := jsonx.Marshal(value)
	if err != nil {
		return err
	}
	return c.Set(ctx, key, string(b), ttl)
}

// Get loads a JSON value through the cache abstraction.
func Get(ctx context.Context, c Cache, key string, dst any) error {
	if c == nil {
		c = Default()
	}
	value, err := c.Get(ctx, key)
	if err != nil {
		return err
	}
	if value == "" {
		return ErrNotFound
	}
	if err := jsonx.Unmarshal([]byte(value), dst); err != nil {
		return err
	}
	return nil
}

// Delete removes one key through the cache abstraction.
func Delete(ctx context.Context, c Cache, key string) error {
	if c == nil {
		c = Default()
	}
	return c.Del(ctx, key)
}

// SetJSON stores value as JSON. Kept for readability at call sites.
func SetJSON(ctx context.Context, c Cache, key string, value any, ttl time.Duration) error {
	return Set(ctx, c, key, value, ttl)
}

// GetJSON loads a JSON value and reports whether the key existed.
func GetJSON(ctx context.Context, c Cache, key string, dst any) (bool, error) {
	err := Get(ctx, c, key, dst)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return false, err
}
