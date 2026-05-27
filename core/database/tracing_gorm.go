package database

import (
	"context"
	"errors"

	"github.com/huwenlong92/sdkit/core/tracing"

	"gorm.io/gorm"
)

const gormSpanKey = "sdkitgo:gorm:span"

func instrumentGorm(db *gorm.DB) error {
	return InstrumentGorm(db)
}

func InstrumentGorm(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	err := db.Use(gormTracingPlugin{})
	if errors.Is(err, gorm.ErrRegistered) {
		return nil
	}
	return err
}

type gormTracingPlugin struct{}

func (gormTracingPlugin) Name() string {
	return "sdkitgo:database:gormtracing"
}

func (gormTracingPlugin) Initialize(db *gorm.DB) error {
	callbacks := []struct {
		before func(string) gormRegister
		after  func(string) gormRegister
		op     string
	}{
		{
			func(name string) gormRegister { return db.Callback().Create().Before(name) },
			func(name string) gormRegister { return db.Callback().Create().After(name) },
			"create",
		},
		{
			func(name string) gormRegister { return db.Callback().Query().Before(name) },
			func(name string) gormRegister { return db.Callback().Query().After(name) },
			"query",
		},
		{
			func(name string) gormRegister { return db.Callback().Delete().Before(name) },
			func(name string) gormRegister { return db.Callback().Delete().After(name) },
			"delete",
		},
		{
			func(name string) gormRegister { return db.Callback().Update().Before(name) },
			func(name string) gormRegister { return db.Callback().Update().After(name) },
			"update",
		},
		{
			func(name string) gormRegister { return db.Callback().Row().Before(name) },
			func(name string) gormRegister { return db.Callback().Row().After(name) },
			"row",
		},
		{
			func(name string) gormRegister { return db.Callback().Raw().Before(name) },
			func(name string) gormRegister { return db.Callback().Raw().After(name) },
			"raw",
		},
	}

	for _, callback := range callbacks {
		if err := callback.before("gorm:"+callback.op).Register("sdkitgo:database:gormtracing:before:"+callback.op, beforeGorm(callback.op)); err != nil {
			return err
		}
		if err := callback.after("gorm:"+callback.op).Register("sdkitgo:database:gormtracing:after:"+callback.op, afterGorm()); err != nil {
			return err
		}
	}
	return nil
}

type gormSpanState struct {
	parent context.Context
	span   tracing.Span
}

type gormRegister interface {
	Register(name string, fn func(*gorm.DB)) error
}

func beforeGorm(operation string) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		ctx := tx.Statement.Context
		if ctx == nil {
			ctx = context.Background()
		}
		if tracing.TraceID(ctx) == "" {
			return
		}

		attrs := []tracing.Attr{
			tracing.String("db.system", "postgresql"),
			tracing.String("db.operation", operation),
		}
		if tx.Statement.Table != "" {
			attrs = append(attrs, tracing.String("db.sql.table", tx.Statement.Table))
		}

		parent := ctx
		ctx, span := tracing.StartSpanWithOptions(parent, "gorm."+operation, tracing.SpanOptions{
			TracerName: "sdkitgo/core/database",
			Kind:       tracing.SpanKindInternal,
		}, attrs...)
		tracing.SetSpanCorrelationAttributes(ctx, span)
		tx.Statement.Context = ctx
		tx.InstanceSet(gormSpanKey, gormSpanState{parent: parent, span: span})
	}
}

func afterGorm() func(*gorm.DB) {
	return func(tx *gorm.DB) {
		value, ok := tx.InstanceGet(gormSpanKey)
		if !ok {
			return
		}
		state, ok := value.(gormSpanState)
		if !ok || state.span == nil {
			return
		}
		defer state.span.End()
		if state.parent != nil {
			defer func() {
				tx.Statement.Context = state.parent
			}()
		}

		if tx.Statement.SQL.Len() > 0 {
			state.span.SetAttributes(tracing.String("db.statement", tx.Statement.SQL.String()))
		}
		if tx.Statement.RowsAffected >= 0 {
			state.span.SetAttributes(tracing.Int64("db.rows_affected", tx.Statement.RowsAffected))
		}
		if tx.Error != nil && !errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			state.span.RecordError(tx.Error)
			state.span.SetStatus(tracing.StatusError, tx.Error.Error())
		}
	}
}
