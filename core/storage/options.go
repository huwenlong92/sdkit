package storage

import (
	"time"

	pkgfs "github.com/huwenlong92/sdkit/pkg/storage"
)

func WithTempDir(tempDir string) Option {
	return pkgfs.WithTempDir(tempDir)
}

func WithMaxSize(maxSize int64) Option {
	return pkgfs.WithMaxSize(maxSize)
}

func WithChunkSize(chunkSize int64) Option {
	return pkgfs.WithChunkSize(chunkSize)
}

func WithTokenTTL(ttl time.Duration) Option {
	return pkgfs.WithTokenTTL(ttl)
}

func WithNameRules(dirRule, fileRule string) Option {
	return pkgfs.WithNameRules(dirRule, fileRule)
}

func WithAllowedExtensions(exts ...string) Option {
	return pkgfs.WithAllowedExtensions(exts...)
}
