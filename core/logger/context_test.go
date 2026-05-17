package logger

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracking"

	"go.uber.org/zap"
)

func TestContextFieldsIncludeCorrelationAndRuntimeFields(t *testing.T) {
	ctx := context.Background()
	ctx = tracking.WithTrackID(ctx, "track-1")
	ctx = WithField(ctx, TraceIDKey, "trace-1")
	ctx = WithField(ctx, SpanIDKey, "span-1")
	ctx = requestid.WithRequestID(ctx, "request-1")
	ctx = WithField(ctx, TaskIDKey, "task-1")
	ctx = WithField(ctx, QueueKey, "critical")
	ctx = WithField(ctx, TypeKey, "user:sync")
	ctx = WithField(ctx, RunIDKey, "run-1")
	ctx = WithField(ctx, JobIDKey, "job-1")

	fields := ContextFields(ctx)
	assertField(t, fields, TrackIDKey, "track-1")
	assertField(t, fields, TraceIDKey, "trace-1")
	assertField(t, fields, SpanIDKey, "span-1")
	assertField(t, fields, RequestIDKey, "request-1")
	assertField(t, fields, TaskIDKey, "task-1")
	assertField(t, fields, QueueKey, "critical")
	assertField(t, fields, TypeKey, "user:sync")
	assertField(t, fields, RunIDKey, "run-1")
	assertField(t, fields, JobIDKey, "job-1")
}

func TestWithFieldUsesTypedKey(t *testing.T) {
	ctx := WithField(context.Background(), TraceIDKey, "trace-typed")

	if got := Field(ctx, TraceIDKey); got != "trace-typed" {
		t.Fatalf("Field() = %q, want trace-typed", got)
	}
	assertField(t, ContextFields(ctx), TraceIDKey, "trace-typed")
}

func assertField(t *testing.T, fields []zap.Field, key, want string) {
	t.Helper()
	for _, field := range fields {
		if field.Key == key && field.String == want {
			return
		}
	}
	t.Fatalf("missing field %s=%s: %+v", key, want, fields)
}
