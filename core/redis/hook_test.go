package redis

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"

	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"
)

func TestHookCreatesRedisSpanWithCorrelation(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	oldProvider := otel.GetTracerProvider()
	oldPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(tracing.NewPropagator())
	defer otel.SetTracerProvider(oldProvider)
	defer otel.SetTextMapPropagator(oldPropagator)
	defer provider.Shutdown(context.Background())

	ctx := tracking.WithTrackID(context.Background(), "track-redis")
	ctx = requestid.WithRequestID(ctx, "request-redis")
	ctx, parent := tracing.StartSpan(ctx, "parent")

	hook := NewHook(zap.NewNop())
	process := hook.ProcessHook(func(ctx context.Context, cmd goredis.Cmder) error {
		if tracing.TraceID(ctx) == "" {
			t.Fatal("redis hook context should contain trace_id")
		}
		return nil
	})
	if err := process(ctx, goredis.NewStringCmd(ctx, "get", "cache:key")); err != nil {
		t.Fatalf("process hook: %v", err)
	}
	parent.End()

	var redisSpan sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "redis.get" {
			redisSpan = span
			break
		}
	}
	if redisSpan == nil {
		t.Fatalf("missing redis span: %+v", recorder.Ended())
	}
	assertRedisSpanAttr(t, redisSpan, "db.system", "redis")
	assertRedisSpanAttr(t, redisSpan, "db.operation", "get")
	assertRedisSpanAttr(t, redisSpan, "track_id", "track-redis")
	assertRedisSpanAttr(t, redisSpan, "request_id", "request-redis")
	assertRedisSpanAttrNotEmpty(t, redisSpan, "traceparent")
}

func TestHookSkipsRedisSpanWithoutParent(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	oldProvider := otel.GetTracerProvider()
	oldPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(tracing.NewPropagator())
	defer otel.SetTracerProvider(oldProvider)
	defer otel.SetTextMapPropagator(oldPropagator)
	defer provider.Shutdown(context.Background())

	hook := NewHook(zap.NewNop())
	process := hook.ProcessHook(func(context.Context, goredis.Cmder) error {
		return nil
	})
	if err := process(context.Background(), goredis.NewStringCmd(context.Background(), "get", "cache:key")); err != nil {
		t.Fatalf("process hook: %v", err)
	}
	if len(recorder.Ended()) != 0 {
		t.Fatalf("redis span should be skipped without parent: %+v", recorder.Ended())
	}
}

func assertRedisSpanAttr(t *testing.T, span sdktrace.ReadOnlySpan, key string, want string) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key && attr.Value.AsString() == want {
			return
		}
	}
	t.Fatalf("missing span attr %s=%s: %+v", key, want, span.Attributes())
}

func assertRedisSpanAttrNotEmpty(t *testing.T, span sdktrace.ReadOnlySpan, key string) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key && attr.Value.AsString() != "" {
			return
		}
	}
	t.Fatalf("missing non-empty span attr %s: %+v", key, span.Attributes())
}
