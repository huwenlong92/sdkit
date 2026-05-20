package storage

import "errors"

var (
	ErrNotConfigured   = errors.New("storage: not configured")
	ErrDefaultRequired = errors.New("storage: default store is required")
	ErrStoreNotFound   = errors.New("storage: store not found")
)
