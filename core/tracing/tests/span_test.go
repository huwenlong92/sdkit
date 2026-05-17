package tests

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/tracing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestStartSpan(t *testing.T) {
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	oldProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(oldProvider)
	defer provider.Shutdown(context.Background())

	ctx, span := tracing.StartSpan(context.Background(), "risk.check", attribute.String("risk.type", "login"))
	defer span.End()

	if tracing.TraceID(ctx) == "" {
		t.Fatal("trace_id should not be empty")
	}
	if tracing.SpanID(ctx) == "" {
		t.Fatal("span_id should not be empty")
	}
}
