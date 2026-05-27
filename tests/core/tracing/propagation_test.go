package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"
)

func TestInjectAndExtractHTTPHeader(t *testing.T) {
	ctx := tracking.WithTrackID(context.Background(), "track-propagation")
	ctx = requestid.WithRequestID(ctx, "request-propagation")
	ctx, span := tracing.StartSpan(ctx, "test.propagation")
	defer span.End()

	header := http.Header{}
	tracing.InjectHTTPHeader(ctx, header)

	if got := header.Get("traceparent"); got == "" {
		t.Fatal("traceparent should not be empty")
	}
	if got := header.Get(tracking.Header); got != "track-propagation" {
		t.Fatalf("track header: want track-propagation, got %s", got)
	}
	if got := header.Get(requestid.Header); got != "request-propagation" {
		t.Fatalf("request header: want request-propagation, got %s", got)
	}

	extracted := tracing.ExtractHTTPHeader(context.Background(), header)
	if tracing.TraceID(extracted) != tracing.TraceID(ctx) {
		t.Fatalf("trace_id: want %s, got %s", tracing.TraceID(ctx), tracing.TraceID(extracted))
	}
	if tracking.TrackID(extracted) != "track-propagation" {
		t.Fatalf("track_id: want track-propagation, got %s", tracking.TrackID(extracted))
	}
	if got := tracing.RequestID(extracted); got != "request-propagation" {
		t.Fatalf("request_id: want request-propagation, got %s", got)
	}
}

func TestMapHeaderCorrelationHelpers(t *testing.T) {
	headers := map[string]string{
		"x-track-id":   "track-map",
		"x-request-id": "request-map",
		"traceparent":  "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01",
	}

	ctx := tracing.ContextFromHeaders(context.Background(), headers)
	if got := tracking.TrackID(ctx); got != "track-map" {
		t.Fatalf("track_id: want track-map, got %s", got)
	}
	if got := tracing.RequestID(ctx); got != "request-map" {
		t.Fatalf("request_id: want request-map, got %s", got)
	}
	if traceID := logger.Field(ctx, logger.TraceIDKey); traceID != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("logger trace_id: got %s", traceID)
	}
	if spanID := logger.Field(ctx, logger.SpanIDKey); spanID != "bbbbbbbbbbbbbbbb" {
		t.Fatalf("logger span_id: got %s", spanID)
	}
}
