package database

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/tracing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
)

func openPGX(ctx context.Context, cfg Config, mode string) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, err
	}

	poolCfg.MaxConns = int32(cfg.MaxOpenConns)
	poolCfg.MinConns = int32(cfg.MaxIdleConns)
	poolCfg.MinIdleConns = int32(cfg.MaxIdleConns)
	if cfg.Schema != "" {
		if poolCfg.ConnConfig.RuntimeParams == nil {
			poolCfg.ConnConfig.RuntimeParams = map[string]string{}
		}
		poolCfg.ConnConfig.RuntimeParams["search_path"] = cfg.Schema
	}
	poolCfg.MaxConnLifetime = cfg.connMaxLifetime()
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	poolCfg.ConnConfig.Tracer = &tracelog.TraceLog{
		Logger:   newPGXLogger(),
		LogLevel: pgxLogLevel(cfg.LogLevel, mode),
	}
	if tracing.Enabled() {
		if err := tracing.InstrumentPgxPoolConfig(poolCfg); err != nil {
			return nil, err
		}
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
