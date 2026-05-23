package tests

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/huwenlong92/sdkit/core/eventbus"
	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestEventFlowHeaderKeysUseSharedCorrelationConstants(t *testing.T) {
	keys := eventbus.EventFlowHeaderKeys()
	want := map[string]bool{
		eventbus.HeaderTraceparent:  true,
		eventbus.HeaderTracestate:   true,
		eventbus.HeaderBaggage:      true,
		logger.TraceIDKey:           true,
		logger.SpanIDKey:            true,
		tracking.Header:             true,
		requestid.Header:            true,
		eventbus.HeaderConnectionID: true,
		eventbus.HeaderSessionID:    true,
	}
	for _, key := range keys {
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing eventflow header keys: %+v", want)
	}

	keys[0] = "mutated"
	if eventbus.EventFlowHeaderKeys()[0] == "mutated" {
		t.Fatal("EventFlowHeaderKeys should return a copy")
	}
}

func TestConnectionAndSessionIDsStayInEventHeaders(t *testing.T) {
	event, err := eventbus.NewJSONEvent(
		context.Background(),
		"rt:events",
		map[string]string{"ok": "1"},
		map[string]string{
			eventbus.HeaderConnectionID: "conn-1",
			eventbus.HeaderSessionID:    "session-1",
		},
	)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if got := event.Headers[eventbus.HeaderConnectionID]; got != "conn-1" {
		t.Fatalf("connection_id header: want conn-1, got %q", got)
	}
	if got := event.Headers[eventbus.HeaderSessionID]; got != "session-1" {
		t.Fatalf("session_id header: want session-1, got %q", got)
	}

	eventType := reflect.TypeOf(eventbus.Event{})
	if _, ok := eventType.FieldByName("ConnectionID"); ok {
		t.Fatal("eventbus.Event must not expose ConnectionID as a top-level field")
	}
	if _, ok := eventType.FieldByName("SessionID"); ok {
		t.Fatal("eventbus.Event must not expose SessionID as a top-level field")
	}
	for i := 0; i < eventType.NumField(); i++ {
		tag := eventType.Field(i).Tag.Get("json")
		tagName, _, _ := strings.Cut(tag, ",")
		if tagName == eventbus.HeaderConnectionID || tagName == eventbus.HeaderSessionID {
			t.Fatalf("eventbus.Event must not expose %s as a top-level json field", tagName)
		}
	}
}

func TestNewEventUsesHeadersWithoutMixingTrackIDIntoTraceID(t *testing.T) {
	oldPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(tracing.NewPropagator())
	defer otel.SetTextMapPropagator(oldPropagator)

	ctx := tracking.WithTrackID(context.Background(), "track-event")
	ctx = requestid.WithRequestID(ctx, "request-event")

	event, err := eventbus.NewJSONEvent(ctx, "rt:events", map[string]string{"ok": "1"}, nil)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if got := tracing.HeaderValue(event.Headers, tracking.Header); got != "track-event" {
		t.Fatalf("track header: want track-event, got %q", got)
	}
	if got := tracing.HeaderValue(event.Headers, requestid.Header); got != "request-event" {
		t.Fatalf("request header: want request-event, got %q", got)
	}

	handlerCtx := eventbus.ContextWithEvent(context.Background(), event)
	if got := tracking.TrackID(handlerCtx); got != "track-event" {
		t.Fatalf("handler track_id: want track-event, got %q", got)
	}
	if got := tracing.RequestID(handlerCtx); got != "request-event" {
		t.Fatalf("handler request_id: want request-event, got %q", got)
	}
	if got, _ := handlerCtx.Value(logger.TraceIDKey).(string); got != "" {
		t.Fatalf("handler trace_id should stay empty without traceparent, got %q", got)
	}
}

func TestNewEventPropagatesTraceHeaders(t *testing.T) {
	restore := installTracing(t)
	defer restore()

	ctx := tracking.WithTrackID(context.Background(), "track-event")
	ctx = requestid.WithRequestID(ctx, "request-event")
	ctx, span := tracing.StartSpan(ctx, "eventbus.publish")
	defer span.End()

	event, err := eventbus.NewJSONEvent(ctx, "rt:events", map[string]string{"ok": "1"}, nil)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	wantTraceID := tracing.TraceID(ctx)
	if got := tracing.HeaderValue(event.Headers, "traceparent"); got == "" {
		t.Fatal("traceparent should not be empty")
	}
	if got := tracing.TraceIDFromHeaders(event.Headers); got != wantTraceID {
		t.Fatalf("trace_id from headers: want %s, got %s", wantTraceID, got)
	}
	if got := tracing.SpanIDFromHeaders(event.Headers); got == "" {
		t.Fatal("span_id from headers should not be empty")
	}

	handlerCtx := eventbus.ContextWithEvent(context.Background(), event)
	if got := tracing.TraceID(handlerCtx); got != wantTraceID {
		t.Fatalf("handler trace_id: want %s, got %s", wantTraceID, got)
	}
	if got := tracking.TrackID(handlerCtx); got != "track-event" {
		t.Fatalf("handler track_id: want track-event, got %q", got)
	}
	if got := tracing.RequestID(handlerCtx); got != "request-event" {
		t.Fatalf("handler request_id: want request-event, got %q", got)
	}
}

func TestContextWithEventDoesNotInventTraceContext(t *testing.T) {
	ctx := eventbus.ContextWithEvent(context.Background(), &eventbus.Event{})
	if got := tracking.TrackID(ctx); got != "" {
		t.Fatalf("event trace_id should not become track_id, got %q", got)
	}
	if got := logger.Field(ctx, logger.TraceIDKey); got != "" {
		t.Fatalf("event trace_id should not be injected into logger context, got %q", got)
	}
}

func installTracing(t *testing.T) func() {
	t.Helper()

	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	oldProvider := otel.GetTracerProvider()
	oldPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(tracing.NewPropagator())

	return func() {
		otel.SetTracerProvider(oldProvider)
		otel.SetTextMapPropagator(oldPropagator)
		_ = provider.Shutdown(context.Background())
	}
}
