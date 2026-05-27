package database

import (
	"context"

	"github.com/huwenlong92/sdkit/core/tracing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/multitracer"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgxSpanKey struct{}

func instrumentPgxPoolConfig(cfg *pgxpool.Config) error {
	return InstrumentPgxPoolConfig(cfg)
}

func InstrumentPgxPoolConfig(cfg *pgxpool.Config) error {
	if cfg == nil || cfg.ConnConfig == nil {
		return nil
	}
	tracer := pgxTracing{}
	if cfg.ConnConfig.Tracer == nil {
		cfg.ConnConfig.Tracer = tracer
		return nil
	}
	cfg.ConnConfig.Tracer = multitracer.New(cfg.ConnConfig.Tracer, tracer)
	return nil
}

type pgxTracing struct{}

func (pgxTracing) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return startPGXSpan(ctx, "pgx.query",
		tracing.String("db.system", "postgresql"),
		tracing.String("db.operation", "query"),
		tracing.String("db.statement", data.SQL),
	)
}

func (pgxTracing) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	endPGXSpan(ctx, data.Err)
}

func (pgxTracing) TraceBatchStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchStartData) context.Context {
	return startPGXSpan(ctx, "pgx.batch",
		tracing.String("db.system", "postgresql"),
		tracing.String("db.operation", "batch"),
	)
}

func (pgxTracing) TraceBatchQuery(ctx context.Context, _ *pgx.Conn, data pgx.TraceBatchQueryData) {
	if span := pgxSpan(ctx); span != nil && data.Err != nil {
		span.RecordError(data.Err)
		span.SetStatus(tracing.StatusError, data.Err.Error())
	}
}

func (pgxTracing) TraceBatchEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceBatchEndData) {
	endPGXSpan(ctx, data.Err)
}

func (pgxTracing) TraceCopyFromStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceCopyFromStartData) context.Context {
	return startPGXSpan(ctx, "pgx.copy_from",
		tracing.String("db.system", "postgresql"),
		tracing.String("db.operation", "copy_from"),
		tracing.String("db.sql.table", data.TableName.Sanitize()),
	)
}

func (pgxTracing) TraceCopyFromEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceCopyFromEndData) {
	endPGXSpan(ctx, data.Err)
}

func (pgxTracing) TracePrepareStart(ctx context.Context, _ *pgx.Conn, data pgx.TracePrepareStartData) context.Context {
	return startPGXSpan(ctx, "pgx.prepare",
		tracing.String("db.system", "postgresql"),
		tracing.String("db.operation", "prepare"),
		tracing.String("db.statement", data.SQL),
		tracing.String("db.statement.name", data.Name),
	)
}

func (pgxTracing) TracePrepareEnd(ctx context.Context, _ *pgx.Conn, data pgx.TracePrepareEndData) {
	endPGXSpan(ctx, data.Err)
}

func startPGXSpan(ctx context.Context, name string, attrs ...tracing.Attr) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if tracing.TraceID(ctx) == "" {
		return ctx
	}
	ctx, span := tracing.StartSpanWithOptions(ctx, name, tracing.SpanOptions{
		TracerName: "sdkitgo/core/database",
		Kind:       tracing.SpanKindInternal,
	}, attrs...)
	tracing.SetSpanCorrelationAttributes(ctx, span)
	return context.WithValue(ctx, pgxSpanKey{}, span)
}

func endPGXSpan(ctx context.Context, err error) {
	span := pgxSpan(ctx)
	if span == nil {
		return
	}
	defer span.End()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(tracing.StatusError, err.Error())
	}
}

func pgxSpan(ctx context.Context) tracing.Span {
	if ctx == nil {
		return nil
	}
	span, _ := ctx.Value(pgxSpanKey{}).(tracing.Span)
	return span
}
