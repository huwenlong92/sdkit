//go:build sdkit_tracing_otel

package otel

import (
	"context"
	"errors"

	"github.com/huwenlong92/sdkit/core/database"
	pkgtracing "github.com/huwenlong92/sdkit/pkg/tracing"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/multitracer"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

const gormSpanKey = "sdkitgo:otelgorm:span"

func init() {
	database.RegisterGormInstrumenter(func(db *gorm.DB) error {
		if !pkgtracing.Enabled() {
			return nil
		}
		return InstrumentGorm(db)
	})
	database.RegisterPgxPoolConfigInstrumenter(func(cfg *pgxpool.Config) error {
		if !pkgtracing.Enabled() {
			return nil
		}
		return InstrumentPgxPoolConfig(cfg)
	})
}

func InstrumentGorm(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	err := db.Use(gormPlugin{})
	if errors.Is(err, gorm.ErrRegistered) {
		return nil
	}
	return err
}

func InstrumentPgxPoolConfig(cfg *pgxpool.Config) error {
	if cfg == nil || cfg.ConnConfig == nil {
		return nil
	}
	otelTracer := otelpgx.NewTracer(otelpgx.WithTracerProvider(correlationTracerProvider{TracerProvider: otel.GetTracerProvider()}))
	if cfg.ConnConfig.Tracer == nil {
		cfg.ConnConfig.Tracer = otelTracer
		return nil
	}
	cfg.ConnConfig.Tracer = multitracer.New(cfg.ConnConfig.Tracer, otelTracer)
	return nil
}

type gormPlugin struct{}

func (gormPlugin) Name() string {
	return "sdkitgo:otelgorm"
}

func (gormPlugin) Initialize(db *gorm.DB) error {
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
		if err := callback.before("gorm:"+callback.op).Register("sdkitgo:otelgorm:before:"+callback.op, beforeGorm(callback.op)); err != nil {
			return err
		}
		if err := callback.after("gorm:"+callback.op).Register("sdkitgo:otelgorm:after:"+callback.op, afterGorm()); err != nil {
			return err
		}
	}
	return nil
}

type gormSpanState struct {
	parent context.Context
	span   oteltrace.Span
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
		if !hasValidParentSpan(ctx) {
			return
		}
		attrs := []attribute.KeyValue{
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", operation),
		}
		if tx.Statement.Table != "" {
			attrs = append(attrs, attribute.String("db.sql.table", tx.Statement.Table))
		}

		parent := ctx
		ctx, span := otel.Tracer("sdkitgo/core/tracing").Start(parent, "gorm."+operation, oteltrace.WithAttributes(attrs...))
		ctx = contextWithSpan(ctx, span)
		setSpanCorrelationAttributes(ctx, span)
		tx.Statement.Context = ctx
		tx.InstanceSet(gormSpanKey, gormSpanState{parent: parent, span: span})
	}
}

type correlationTracerProvider struct {
	oteltrace.TracerProvider
}

func (p correlationTracerProvider) Tracer(name string, opts ...oteltrace.TracerOption) oteltrace.Tracer {
	delegate := p.TracerProvider
	if delegate == nil {
		delegate = otel.GetTracerProvider()
	}
	return correlationTracer{Tracer: delegate.Tracer(name, opts...)}
}

type correlationTracer struct {
	oteltrace.Tracer
}

func (t correlationTracer) Start(ctx context.Context, name string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	ctx, span := t.Tracer.Start(ctx, name, opts...)
	ctx = contextWithSpan(ctx, span)
	setSpanCorrelationAttributes(ctx, span)
	return ctx, span
}

func hasValidParentSpan(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	return oteltrace.SpanContextFromContext(ctx).IsValid()
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
			state.span.SetAttributes(attribute.String("db.statement", tx.Statement.SQL.String()))
		}
		if tx.Statement.RowsAffected >= 0 {
			state.span.SetAttributes(attribute.Int64("db.rows_affected", tx.Statement.RowsAffected))
		}
		if tx.Error != nil && !errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			state.span.RecordError(tx.Error)
			state.span.SetStatus(codes.Error, tx.Error.Error())
		}
	}
}
