package cache

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/jsonx"
)

// SetJSON stores value as JSON through the default cache.
func SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	return SetJSONWith(ctx, nil, key, value, ttl)
}

// SetJSONWith stores value as JSON through the provided cache.
func SetJSONWith(ctx context.Context, c Cache, key string, value any, ttl time.Duration) error {
	b, err := jsonx.Marshal(value)
	if err != nil {
		return err
	}
	return SetWith(ctx, c, key, string(b), ttl)
}

// GetJSON loads a JSON value from the default cache and reports whether the key existed.
func GetJSON(ctx context.Context, key string, dst any) (bool, error) {
	return GetJSONWith(ctx, nil, key, dst)
}

// GetJSONWith loads a JSON value from the provided cache and reports whether the key existed.
func GetJSONWith(ctx context.Context, c Cache, key string, dst any) (bool, error) {
	err := readJSON(ctx, c, key, dst)
	if err == nil {
		return true, nil
	}
	if IsNotFound(err) {
		return false, nil
	}
	return false, err
}

// DeleteJSON removes one JSON cache key from the default cache.
func DeleteJSON(ctx context.Context, key string) error {
	return Del(ctx, key)
}

// DeleteJSONWith removes one JSON cache key from the provided cache.
func DeleteJSONWith(ctx context.Context, c Cache, key string) error {
	return DelWith(ctx, c, key)
}

func readJSON(ctx context.Context, c Cache, key string, dst any) error {
	value, err := GetWith(ctx, c, key)
	if err != nil {
		return err
	}
	if value == "" {
		return ErrNotFound
	}
	return jsonx.Unmarshal([]byte(value), dst)
}
