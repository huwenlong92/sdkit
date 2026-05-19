package cache

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"time"

	"golang.org/x/sync/singleflight"
)

var rememberGroup singleflight.Group

// Remember returns a cached JSON value from the default cache, or loads and stores it once per key.
func Remember[T any](ctx context.Context, key string, ttl time.Duration, fn func() (T, error)) (T, error) {
	return RememberWith(ctx, nil, key, ttl, fn)
}

// RememberWith returns a cached JSON value from the provided cache, or loads and stores it once per key.
func RememberWith[T any](ctx context.Context, c Cache, key string, ttl time.Duration, fn func() (T, error)) (T, error) {
	var zero T
	if c == nil {
		c = Default()
	}

	var cached T
	err := readJSON(ctx, c, key, &cached)
	if err == nil {
		return cached, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return zero, err
	}

	v, err, _ := rememberGroup.Do(rememberFlightKey(c, key), func() (any, error) {
		var cached T
		err := readJSON(ctx, c, key, &cached)
		if err == nil {
			return cached, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, err
		}

		loaded, err := fn()
		if err != nil {
			return nil, err
		}
		if err := SetJSONWith(ctx, c, key, loaded, ttl); err != nil {
			return nil, err
		}
		return loaded, nil
	})
	if err != nil {
		return zero, err
	}

	typed, ok := v.(T)
	if !ok {
		return zero, errors.New("cache remember type mismatch")
	}
	return typed, nil
}

func rememberFlightKey(c Cache, key string) string {
	v := reflect.ValueOf(c)
	if v.IsValid() && v.Kind() == reflect.Pointer {
		return strconv.FormatUint(uint64(v.Pointer()), 16) + ":" + key
	}
	return key
}
