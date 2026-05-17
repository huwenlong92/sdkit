package gormoutbox

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"

	"go.opentelemetry.io/otel"
)

func TestOutboxEnqueueContextRestoresSavedCorrelationHeaders(t *testing.T) {
	oldPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(tracing.NewPropagator())
	defer otel.SetTextMapPropagator(oldPropagator)

	ctx := outboxEnqueueContext(context.Background(), map[string]string{
		"traceparent":    "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01",
		tracking.Header:  "track-outbox",
		requestid.Header: "request-outbox",
	})

	if got := tracing.TraceID(ctx); got != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("trace_id = %q, want aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", got)
	}
	if got := tracking.TrackID(ctx); got != "track-outbox" {
		t.Fatalf("track_id = %q, want track-outbox", got)
	}
	if got := tracing.RequestID(ctx); got != "request-outbox" {
		t.Fatalf("request_id = %q, want request-outbox", got)
	}
}
