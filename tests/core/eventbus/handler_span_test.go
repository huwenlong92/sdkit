//go:build sdkit_tracing

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/eventbus"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"
	"github.com/huwenlong92/sdkit/pkg/eventbus/memory"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracingMiddlewareCreatesHandlerSpan(t *testing.T) {
	recorder, restore := installTracingRecorder(t)
	defer restore()

	bus := memory.New()
	ctx := tracking.WithTrackID(context.Background(), "track-event")
	ctx = requestid.WithRequestID(ctx, "request-event")
	ctx, parent := tracing.StartSpan(ctx, "eventbus.publish")
	defer parent.End()
	parentTraceID := tracing.TraceID(ctx)

	gotCtx := make(chan context.Context, 1)
	if _, err := bus.Subscribe(context.Background(), "rt:events", func(ctx context.Context, event *eventbus.Event) error {
		gotCtx <- ctx
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	published, err := eventbus.NewJSONEvent(ctx, "rt:events", map[string]string{"ok": "1"}, nil)
	if err != nil {
		t.Fatalf("NewJSONEvent: %v", err)
	}
	if err := bus.Publish(ctx, published); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case handlerCtx := <-gotCtx:
		if got := tracing.TraceID(handlerCtx); got != parentTraceID {
			t.Fatalf("handler trace_id: want %s, got %s", parentTraceID, got)
		}
		if got := tracking.TrackID(handlerCtx); got != "track-event" {
			t.Fatalf("handler track_id: want track-event, got %q", got)
		}
		if got := tracing.RequestID(handlerCtx); got != "request-event" {
			t.Fatalf("handler request_id: want request-event, got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("handler was not called")
	}

	span := waitForSpan(t, recorder, "eventbus.handle rt:events")
	if got := span.SpanContext().TraceID().String(); got != parentTraceID {
		t.Fatalf("handler span trace_id: want %s, got %s", parentTraceID, got)
	}
}

func installTracingRecorder(t *testing.T) (*tracetest.SpanRecorder, func()) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	oldProvider := otel.GetTracerProvider()
	oldPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	_, _ = tracing.Init(context.Background(), tracing.Config{})

	return recorder, func() {
		otel.SetTracerProvider(oldProvider)
		otel.SetTextMapPropagator(oldPropagator)
		_ = provider.Shutdown(context.Background())
	}
}

func waitForSpan(t *testing.T, recorder *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, span := range recorder.Ended() {
			if span.Name() == name {
				return span
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("missing span %q; ended=%d", name, len(recorder.Ended()))
	return nil
}
