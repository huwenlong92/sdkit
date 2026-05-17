package database

import (
	"context"

	"github.com/huwenlong92/sdkit/core/runtime"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
)

const KeyDatabase runtime.Key = "database"

func From(app *runtime.App) *Database {
	if app != nil {
		if value, ok := app.Container().Get(KeyDatabase); ok {
			if db, ok := value.(*Database); ok {
				return db
			}
		}
	}
	return Default
}

func Bind(app *runtime.App, db *Database) error {
	if db == nil {
		setDefault(nil)
		if app == nil {
			return nil
		}
		return runtime.ErrContainerValueNil
	}
	setDefault(db)
	if app == nil {
		return nil
	}
	return app.Container().Bind(KeyDatabase, db)
}

func GormFrom(app *runtime.App) *gorm.DB {
	if db := From(app); db != nil {
		return db.Gorm
	}
	return DB
}

func Gorm(ctxs ...context.Context) *gorm.DB {
	db := GormFrom(nil)
	if db == nil {
		return nil
	}
	ctx := context.Background()
	if len(ctxs) > 0 && ctxs[0] != nil {
		ctx = ctxs[0]
	}
	return db.WithContext(ctx)
}

func PGXFrom(app *runtime.App) *pgxpool.Pool {
	if db := From(app); db != nil {
		return db.PGX
	}
	return PGXPool
}

func PGX(ctxs ...context.Context) *pgxpool.Pool {
	_ = ctxs
	return PGXFrom(nil)
}

func Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	if fn == nil {
		return nil
	}
	db := Gorm(ctx)
	if db == nil {
		return ErrNotInitialized
	}
	return db.Transaction(fn)
}

func PGXTransaction(ctx context.Context, fn func(tx pgx.Tx) error) error {
	if fn == nil {
		return nil
	}
	pool := PGX(ctx)
	if pool == nil {
		return ErrNotInitialized
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

func setDefault(db *Database) {
	Default = db
	if db == nil {
		DB = nil
		PGXPool = nil
		return
	}
	DB = db.Gorm
	PGXPool = db.PGX
}
