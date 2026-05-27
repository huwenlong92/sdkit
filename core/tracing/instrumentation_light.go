//go:build !sdkit_tracing

package tracing

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
)

func InstrumentGorm(*gorm.DB) error {
	return ErrNotCompiled
}

func InstrumentPgxPoolConfig(*pgxpool.Config) error {
	return ErrNotCompiled
}
