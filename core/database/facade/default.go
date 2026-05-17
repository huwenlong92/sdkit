package database

import (
	"context"

	coredatabase "github.com/huwenlong92/sdkit/core/database"
	"github.com/huwenlong92/sdkit/core/runtime"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
)

func From(app *runtime.App) *Database {
	return coredatabase.From(app)
}

func GormFrom(app *runtime.App) *gorm.DB {
	return coredatabase.GormFrom(app)
}

func Gorm(ctxs ...context.Context) *gorm.DB {
	return coredatabase.Gorm(ctxs...)
}

func PGXFrom(app *runtime.App) *pgxpool.Pool {
	return coredatabase.PGXFrom(app)
}

func PGX(ctxs ...context.Context) *pgxpool.Pool {
	return coredatabase.PGX(ctxs...)
}

func Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return coredatabase.Transaction(ctx, fn)
}

func PGXTransaction(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return coredatabase.PGXTransaction(ctx, fn)
}
