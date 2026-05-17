package database

import (
	"context"

	coredatabase "github.com/huwenlong92/sdkit/core/database"
)

func New(ctx context.Context, cfg Config, mode string) (*Database, error) {
	return coredatabase.New(ctx, cfg, mode)
}

func Init(cfg Config, mode string) error {
	return coredatabase.Init(cfg, mode)
}

func InitWithConfig(cfg Config, mode string) error {
	return coredatabase.InitWithConfig(cfg, mode)
}

func Close() {
	coredatabase.Close()
}
