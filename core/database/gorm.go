package database

import (
	"time"

	"github.com/huwenlong92/sdkit/core/tracing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func openGORM(cfg Config, mode string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   cfg.TablePrefix,
			SingularTable: true,
		},
		Logger: newGORMLogger(cfg.LogLevel, mode),
	})
	if err != nil {
		return nil, err
	}
	if tracing.Enabled() {
		if err := tracing.InstrumentGorm(db); err != nil {
			return nil, err
		}
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.connMaxLifetime())
	sqlDB.SetConnMaxIdleTime(30 * time.Minute)

	return db, nil
}
