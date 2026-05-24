package storage

import (
	"time"

	"github.com/huwenlong92/sdkit/pkg/storage/core"
)

const (
	defaultChunkSize = int64(5 << 20)
	defaultTokenTTL  = 2 * time.Hour
)

func normalizeConfig(cfg core.Config) core.Config {
	cfg.Policy.Driver = firstNonEmpty(cfg.Policy.Driver, cfg.Driver)
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = defaultChunkSize
	}
	if cfg.TokenTTL <= 0 {
		cfg.TokenTTL = defaultTokenTTL
	}
	if cfg.DirRule == "" {
		cfg.DirRule = "uploads/{date}"
	}
	if cfg.FileRule == "" {
		cfg.FileRule = "{originname}{ext}"
	}
	return cfg
}
