package database

import (
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
)

type GormInstrumenter func(*gorm.DB) error
type PgxPoolConfigInstrumenter func(*pgxpool.Config) error

var instrumenters = struct {
	sync.RWMutex
	gorm []GormInstrumenter
	pgx  []PgxPoolConfigInstrumenter
}{}

func RegisterGormInstrumenter(instrumenter GormInstrumenter) {
	if instrumenter == nil {
		return
	}
	instrumenters.Lock()
	instrumenters.gorm = append(instrumenters.gorm, instrumenter)
	instrumenters.Unlock()
}

func RegisterPgxPoolConfigInstrumenter(instrumenter PgxPoolConfigInstrumenter) {
	if instrumenter == nil {
		return
	}
	instrumenters.Lock()
	instrumenters.pgx = append(instrumenters.pgx, instrumenter)
	instrumenters.Unlock()
}

func instrumentGorm(db *gorm.DB) error {
	instrumenters.RLock()
	list := append([]GormInstrumenter(nil), instrumenters.gorm...)
	instrumenters.RUnlock()
	for _, instrumenter := range list {
		if err := instrumenter(db); err != nil {
			return err
		}
	}
	return nil
}

func instrumentPgxPoolConfig(cfg *pgxpool.Config) error {
	instrumenters.RLock()
	list := append([]PgxPoolConfigInstrumenter(nil), instrumenters.pgx...)
	instrumenters.RUnlock()
	for _, instrumenter := range list {
		if err := instrumenter(cfg); err != nil {
			return err
		}
	}
	return nil
}
