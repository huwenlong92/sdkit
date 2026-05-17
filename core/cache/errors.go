package cache

import "errors"

// ErrNotFound is returned by typed cache helpers when a key is missing.
var ErrNotFound = errors.New("cache not found")

// IsNotFound reports whether err represents a cache miss.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
