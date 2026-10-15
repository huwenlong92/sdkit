package database

import (
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func openGORM(cfg Config, mode string) (*gorm.DB, error) {
	var dialector gorm.Dialector = postgres.Open(cfg.DSN)
	if cfg.Driver == "sqlite" {
		dialector = sqlite.Open(cfg.DSN)
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   cfg.TablePrefix,
			SingularTable: true,
		},
		Logger: newGORMLogger(cfg.LogLevel, mode),
	})
	if err != nil {
		return nil, err
	}
	if err := instrumentGorm(db); err != nil {
		return nil, err
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
