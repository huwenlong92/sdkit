//go:build sdkit_tracing

package tracing

import (
	providerotel "github.com/huwenlong92/sdkit/pkg/tracing/provider/otel"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
)

func InstrumentGorm(db *gorm.DB) error {
	return providerotel.InstrumentGorm(db)
}

func InstrumentPgxPoolConfig(cfg *pgxpool.Config) error {
	return providerotel.InstrumentPgxPoolConfig(cfg)
}
