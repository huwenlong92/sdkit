package tests

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/tracing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestTraceIDAndSpanID(t *testing.T) {
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	oldProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(oldProvider)
	defer provider.Shutdown(context.Background())

	ctx, span := tracing.StartSpan(context.Background(), "test.context")
	defer span.End()

	if tracing.TraceID(ctx) == "" {
		t.Fatal("trace_id should not be empty")
	}
	if tracing.SpanID(ctx) == "" {
		t.Fatal("span_id should not be empty")
	}
}

func TestLoggerContextFieldsIncludeTraceAndSpan(t *testing.T) {
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	oldProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(oldProvider)
	defer provider.Shutdown(context.Background())

	ctx, span := tracing.StartSpan(context.Background(), "test.logger")
	defer span.End()

	fields := logger.ContextFields(ctx)
	hasTraceID := false
	hasSpanID := false
	for _, field := range fields {
		if field.Key == logger.TraceIDKey && field.String != "" {
			hasTraceID = true
		}
		if field.Key == logger.SpanIDKey && field.String != "" {
			hasSpanID = true
		}
	}
	if !hasTraceID || !hasSpanID {
		t.Fatalf("missing trace/span fields: %+v", fields)
	}
}
