package database

import (
	"context"
	"errors"

	"github.com/huwenlong92/sdkit/core/logger"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	ErrNotInitialized = errors.New("database: not initialized")

	Default *Database
	DB      *gorm.DB
	PGXPool *pgxpool.Pool
)

type Database struct {
	Gorm   *gorm.DB
	PGX    *pgxpool.Pool
	Config Config
}

func Init(cfg Config, mode string) error {
	return InitWithConfig(cfg, mode)
}

func InitWithConfig(cfg Config, mode string) error {
	logger.L.Info("正在初始化数据库连接")

	db, err := New(context.Background(), cfg, mode)
	if err != nil {
		logger.L.Error("数据库连接失败", zap.Error(err))
		return err
	}

	setDefault(db)

	logger.L.Info("数据库连接成功")
	return nil
}

func New(ctx context.Context, cfg Config, mode string) (*Database, error) {
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	gormDB, err := openGORM(cfg, mode)
	if err != nil {
		return nil, err
	}

	pgxPool, err := openPGX(ctx, cfg, mode)
	if err != nil {
		closeGORM(gormDB)
		return nil, err
	}

	return &Database{Gorm: gormDB, PGX: pgxPool, Config: cfg}, nil
}

func Close() {
	if Default != nil {
		_ = Default.Close()
	}
	setDefault(nil)
}

func (db *Database) Close() error {
	if db == nil {
		return nil
	}
	var errs []error
	if db.PGX != nil {
		db.PGX.Close()
		db.PGX = nil
	}
	if db.Gorm != nil {
		if err := closeGORM(db.Gorm); err != nil {
			errs = append(errs, err)
		}
		db.Gorm = nil
	}
	return errors.Join(errs...)
}

func closeGORM(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
