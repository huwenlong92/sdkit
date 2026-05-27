//go:build sdkit_tracing_otel

package queue_test

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/queue/runtime/middleware"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRuntimeMiddlewareTracingCreatesWorkerSpan(t *testing.T) {
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

	wantErr := errors.New("worker error")
	handler := middleware.Tracing()(func(ctx context.Context, _ *queue.Message) error {
		if tracing.TraceID(ctx) == "" || tracing.SpanID(ctx) == "" {
			t.Fatal("worker tracing middleware should pass span context to handler")
		}
		return wantErr
	})

	ctx := tracking.WithTrackID(context.Background(), "track-queue")
	ctx = requestid.WithRequestID(ctx, "request-queue")
	err := handler(ctx, &queue.Message{
		ID:    "task-id",
		Type:  "user_sync",
		Queue: "critical",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("handler error = %v, want %v", err, wantErr)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}
	if spans[0].Name() != "handler::user_sync" {
		t.Fatalf("span name = %q, want %q", spans[0].Name(), "handler::user_sync")
	}
	if !spanHasAttrValue(spans[0], "trace_id", spans[0].SpanContext().TraceID().String()) {
		t.Fatalf("handler span should have trace_id attribute")
	}
	if !spanHasAttrValue(spans[0], "span_id", spans[0].SpanContext().SpanID().String()) {
		t.Fatalf("handler span should have span_id attribute")
	}
	if !spanHasAttrValue(spans[0], "track_id", "track-queue") {
		t.Fatalf("handler span should have track_id attribute")
	}
	if !spanHasAttrValue(spans[0], "request_id", "request-queue") {
		t.Fatalf("handler span should have request_id attribute")
	}
	if !spanHasAttr(spans[0], "traceparent") {
		t.Fatalf("handler span should have traceparent attribute")
	}
}

func spanHasAttr(span sdktrace.ReadOnlySpan, key string) bool {
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key && attr.Value.AsString() != "" {
			return true
		}
	}
	return false
}

func spanHasAttrValue(span sdktrace.ReadOnlySpan, key string, want string) bool {
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key && attr.Value.AsString() == want {
			return true
		}
	}
	return false
}
