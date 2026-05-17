package filesystem

import (
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/pkg/filesystem/core"
)

const (
	defaultUploadDir = "uploads"
	defaultChunkSize = int64(5 << 20)
	defaultTokenTTL  = 2 * time.Hour
)

func normalizeConfig(cfg core.Config) core.Config {
	cfg.Policy.Driver = firstNonEmpty(cfg.Policy.Driver, cfg.Driver)
	if cfg.UploadDir == "" {
		cfg.UploadDir = defaultUploadDir
	}
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = defaultChunkSize
	}
	if cfg.TokenTTL <= 0 {
		cfg.TokenTTL = defaultTokenTTL
	}
	if cfg.DirRule == "" {
		cfg.DirRule = "{date}"
	}
	if cfg.FileRule == "" {
		cfg.FileRule = "{originname}{ext}"
	}
	cfg.UploadDir = strings.Trim(cfg.UploadDir, "/")
	return cfg
}
