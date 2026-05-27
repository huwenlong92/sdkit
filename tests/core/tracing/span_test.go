package tests

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/tracing"
)

func TestStartSpan(t *testing.T) {
	ctx, span := tracing.StartSpan(context.Background(), "risk.check", tracing.String("risk.type", "login"))
	defer span.End()

	if tracing.TraceID(ctx) == "" {
		t.Fatal("trace_id should not be empty")
	}
	if tracing.SpanID(ctx) == "" {
		t.Fatal("span_id should not be empty")
	}
}
