//go:build sdkit_tracing

package tests

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/multitracer"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestInstrumentGormRegistersPlugin(t *testing.T) {
	db, err := gorm.Open(postgres.Open("host=localhost user=test dbname=test sslmode=disable"), &gorm.Config{
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}

	if err := tracing.InstrumentGorm(db); err != nil {
		t.Fatalf("instrument gorm: %v", err)
	}
	if err := tracing.InstrumentGorm(db); err != nil {
		t.Fatalf("instrument gorm twice: %v", err)
	}
	if _, ok := db.Plugins["sdkitgo:otelgorm"]; !ok {
		t.Fatal("sdkitgo otelgorm plugin should be registered")
	}
}

func TestInstrumentGormCreatesOperationSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	oldProvider := otel.GetTracerProvider()
	oldPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	_, _ = tracing.Init(context.Background(), tracing.Config{})
	defer otel.SetTracerProvider(oldProvider)
	defer otel.SetTextMapPropagator(oldPropagator)
	defer provider.Shutdown(context.Background())

	db, err := gorm.Open(postgres.Open("host=localhost user=test dbname=test sslmode=disable"), &gorm.Config{
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	if err := tracing.InstrumentGorm(db); err != nil {
		t.Fatalf("instrument gorm: %v", err)
	}

	type user struct {
		ID int64
	}
	ctx := tracking.WithTrackID(context.Background(), "track-gorm")
	ctx = requestid.WithRequestID(ctx, "request-gorm")
	ctx, parent := tracing.StartSpan(ctx, "test.gorm")
	err = db.WithContext(ctx).Session(&gorm.Session{DryRun: true}).Where("id = ?", 1).Find(&user{}).Error
	parent.End()
	if err != nil {
		t.Fatalf("dry run query: %v", err)
	}

	var gormSpan sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "gorm.query" {
			gormSpan = span
			break
		}
	}
	if gormSpan == nil {
		t.Fatalf("missing gorm.query span: %+v", recorder.Ended())
	}
	assertSpanAttr(t, gormSpan, "track_id", "track-gorm")
	assertSpanAttr(t, gormSpan, "request_id", "request-gorm")
	assertSpanAttr(t, gormSpan, "trace_id", gormSpan.SpanContext().TraceID().String())
	assertSpanAttrNotEmpty(t, gormSpan, "traceparent")
}

func TestInstrumentGormSkipsOperationSpanWithoutParent(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	oldProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(oldProvider)
	defer provider.Shutdown(context.Background())

	db, err := gorm.Open(postgres.Open("host=localhost user=test dbname=test sslmode=disable"), &gorm.Config{
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	if err := tracing.InstrumentGorm(db); err != nil {
		t.Fatalf("instrument gorm: %v", err)
	}

	type user struct {
		ID int64
	}
	err = db.WithContext(context.Background()).Session(&gorm.Session{DryRun: true}).Where("id = ?", 1).Find(&user{}).Error
	if err != nil {
		t.Fatalf("dry run query: %v", err)
	}

	for _, span := range recorder.Ended() {
		if span.Name() == "gorm.query" {
			t.Fatalf("gorm.query span should be skipped without parent: %+v", recorder.Ended())
		}
	}
}

func TestInstrumentGormRestoresParentContext(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	oldProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(oldProvider)
	defer provider.Shutdown(context.Background())

	db, err := gorm.Open(postgres.Open("host=localhost user=test dbname=test sslmode=disable"), &gorm.Config{
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	if err := tracing.InstrumentGorm(db); err != nil {
		t.Fatalf("instrument gorm: %v", err)
	}

	type user struct {
		ID int64
	}
	ctx, parent := tracing.StartSpan(context.Background(), "test.gorm.parent")
	parentID := parent.SpanID()

	query := db.WithContext(ctx).Session(&gorm.Session{DryRun: true}).Model(&user{})
	var count int64
	if err := query.Count(&count).Error; err != nil {
		t.Fatalf("dry run count: %v", err)
	}
	var users []user
	if err := query.Find(&users).Error; err != nil {
		t.Fatalf("dry run find: %v", err)
	}
	parent.End()

	children := 0
	for _, span := range recorder.Ended() {
		if span.Name() != "gorm.query" {
			continue
		}
		children++
		if span.Parent().SpanID().String() != parentID {
			t.Fatalf("gorm span parent: want %s, got %s", parentID, span.Parent().SpanID())
		}
	}
	if children != 2 {
		t.Fatalf("gorm.query span count: want 2, got %d", children)
	}
}

func TestInstrumentPgxPoolConfigKeepsExistingTracer(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://user:pass@localhost:5432/db?sslmode=disable")
	if err != nil {
		t.Fatalf("parse pgx config: %v", err)
	}
	cfg.ConnConfig.Tracer = &tracelog.TraceLog{Logger: tracelog.LoggerFunc(func(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]any) {})}

	if err := tracing.InstrumentPgxPoolConfig(cfg); err != nil {
		t.Fatalf("instrument pgx: %v", err)
	}
	tracer, ok := cfg.ConnConfig.Tracer.(*multitracer.Tracer)
	if !ok {
		t.Fatalf("tracer should be multitracer, got %T", cfg.ConnConfig.Tracer)
	}
	if len(tracer.QueryTracers) != 2 {
		t.Fatalf("query tracers: want 2, got %d", len(tracer.QueryTracers))
	}
}

func TestInstrumentPgxPoolConfigAddsCorrelationAttributes(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	oldProvider := otel.GetTracerProvider()
	oldPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	_, _ = tracing.Init(context.Background(), tracing.Config{})
	defer otel.SetTracerProvider(oldProvider)
	defer otel.SetTextMapPropagator(oldPropagator)
	defer provider.Shutdown(context.Background())

	cfg, err := pgxpool.ParseConfig("postgres://user:pass@localhost:5432/db?sslmode=disable")
	if err != nil {
		t.Fatalf("parse pgx config: %v", err)
	}
	if err := tracing.InstrumentPgxPoolConfig(cfg); err != nil {
		t.Fatalf("instrument pgx: %v", err)
	}

	ctx := tracking.WithTrackID(context.Background(), "track-pgx")
	ctx = requestid.WithRequestID(ctx, "request-pgx")
	ctx, parent := tracing.StartSpan(ctx, "test.pgx")
	tracer := cfg.ConnConfig.Tracer
	ctx = tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "SELECT 1"})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})
	parent.End()

	var pgxSpan sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() != "test.pgx" {
			pgxSpan = span
			break
		}
	}
	if pgxSpan == nil {
		t.Fatalf("missing pgx span: %+v", recorder.Ended())
	}
	assertSpanAttr(t, pgxSpan, "track_id", "track-pgx")
	assertSpanAttr(t, pgxSpan, "request_id", "request-pgx")
	assertSpanAttr(t, pgxSpan, "trace_id", pgxSpan.SpanContext().TraceID().String())
	assertSpanAttrNotEmpty(t, pgxSpan, "traceparent")
}

func assertSpanAttr(t *testing.T, span sdktrace.ReadOnlySpan, key string, want string) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key && attr.Value.AsString() == want {
			return
		}
	}
	t.Fatalf("missing span attr %s=%s: %+v", key, want, span.Attributes())
}

func assertSpanAttrNotEmpty(t *testing.T, span sdktrace.ReadOnlySpan, key string) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key && attr.Value.AsString() != "" {
			return
		}
	}
	t.Fatalf("missing non-empty span attr %s: %+v", key, span.Attributes())
}
