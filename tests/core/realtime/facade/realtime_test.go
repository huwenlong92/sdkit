package tests

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/eventbus"
	corerealtime "github.com/huwenlong92/sdkit/core/realtime"
	realtimecap "github.com/huwenlong92/sdkit/core/realtime/facade"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"
	"github.com/huwenlong92/sdkit/pkg/eventbus/memory"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestNewSetsDefaultAndPackagePushPublishes(t *testing.T) {
	resetDefaults(t)

	bus := memory.New()
	eventbus.SetDefaultWithDriver(bus, "memory")
	received := subscribeRealtime(t, bus, corerealtime.DefaultTopic)

	capability, err := realtimecap.New(realtimecap.Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if realtimecap.Default() != capability {
		t.Fatal("default realtime capability was not set")
	}
	if capability.Topic() != corerealtime.DefaultTopic {
		t.Fatalf("topic: want %s, got %s", corerealtime.DefaultTopic, capability.Topic())
	}

	ctx := tracking.WithTrackID(context.Background(), "track-cap")
	ctx = requestid.WithRequestID(ctx, "request-cap")
	if err := realtimecap.PushUser(ctx, 1001, "notify.created", map[string]string{"title": "hello"}); err != nil {
		t.Fatalf("PushUser: %v", err)
	}

	event := receiveEvent(t, received)
	if event.Topic != corerealtime.DefaultTopic {
		t.Fatalf("event route: topic=%q", event.Topic)
	}
	if got := tracing.HeaderValue(event.Headers, tracking.Header); got != "track-cap" {
		t.Fatalf("event track header: want track-cap, got %q", got)
	}
	if got := tracing.HeaderValue(event.Headers, requestid.Header); got != "request-cap" {
		t.Fatalf("event request header: want request-cap, got %q", got)
	}

	msg := decodeRealtimeEvent(t, event)
	if msg.Target == nil || msg.Target.Type != corerealtime.TargetUser || msg.Target.UserID != "1001" {
		t.Fatalf("target: want user/1001, got %+v", msg.Target)
	}
	if got := tracing.HeaderValue(msg.Headers, tracking.Header); got != "track-cap" {
		t.Fatalf("realtime track header: want track-cap, got %q", got)
	}
	if got := tracing.HeaderValue(msg.Headers, requestid.Header); got != "request-cap" {
		t.Fatalf("realtime request header: want request-cap, got %q", got)
	}
}

func TestCapabilityPropagatesTraceHeaders(t *testing.T) {
	resetDefaults(t)
	restore := installTracing(t)
	defer restore()

	bus := memory.New()
	received := subscribeRealtime(t, bus, "rt:test")
	capability, err := realtimecap.New(realtimecap.Config{Topic: "rt:test"}, realtimecap.WithEventBus(bus), realtimecap.WithoutDefault())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if realtimecap.Default() != nil {
		t.Fatal("WithoutDefault should not set package default")
	}

	ctx := tracking.WithTrackID(context.Background(), "track-trace")
	ctx = requestid.WithRequestID(ctx, "request-trace")
	ctx, span := tracing.StartSpan(ctx, "realtime.capability.publish")
	defer span.End()
	wantTraceID := tracing.TraceID(ctx)

	if err := capability.PushRoom(ctx, "room-1", corerealtime.NewEvent("room.updated", map[string]bool{"ok": true})); err != nil {
		t.Fatalf("PushRoom: %v", err)
	}

	event := receiveEvent(t, received)
	if got := tracing.TraceIDFromHeaders(event.Headers); got != wantTraceID {
		t.Fatalf("event trace_id from headers: want %s, got %s", wantTraceID, got)
	}
	if got := tracing.HeaderValue(event.Headers, tracking.Header); got != "track-trace" {
		t.Fatalf("event track header: want track-trace, got %q", got)
	}
	if got := tracing.HeaderValue(event.Headers, requestid.Header); got != "request-trace" {
		t.Fatalf("event request header: want request-trace, got %q", got)
	}

	msg := decodeRealtimeEvent(t, event)
	if msg.TraceID != wantTraceID {
		t.Fatalf("realtime trace_id: want %s, got %s", wantTraceID, msg.TraceID)
	}
	if msg.Target == nil || msg.Target.Type != corerealtime.TargetRoom || msg.Target.RoomID != "room-1" {
		t.Fatalf("target: want room/room-1, got %+v", msg.Target)
	}
	if got := tracing.TraceIDFromHeaders(msg.Headers); got != wantTraceID {
		t.Fatalf("realtime trace_id from headers: want %s, got %s", wantTraceID, got)
	}
	if got := tracing.HeaderValue(msg.Headers, tracking.Header); got != "track-trace" {
		t.Fatalf("realtime track header: want track-trace, got %q", got)
	}
	if got := tracing.HeaderValue(msg.Headers, requestid.Header); got != "request-trace" {
		t.Fatalf("realtime request header: want request-trace, got %q", got)
	}
}

func TestNewErrorsWhenEventBusDefaultMissing(t *testing.T) {
	resetDefaults(t)

	_, err := realtimecap.New(realtimecap.Config{})
	if !errors.Is(err, realtimecap.ErrEventBusNotConfigured) {
		t.Fatalf("New: want ErrEventBusNotConfigured, got %v", err)
	}
	if !errors.Is(err, eventbus.ErrDefaultNotInitialized) {
		t.Fatalf("New: want ErrDefaultNotInitialized, got %v", err)
	}
	if realtimecap.Default() != nil {
		t.Fatal("default realtime capability should stay unset after New error")
	}
}

func TestCloseClearsDefaultAndRejectsPush(t *testing.T) {
	resetDefaults(t)

	bus := memory.New()
	capability, err := realtimecap.New(realtimecap.Config{}, realtimecap.WithEventBus(bus))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if realtimecap.Default() != capability {
		t.Fatal("default realtime capability was not set")
	}

	if err := capability.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if realtimecap.Default() != nil {
		t.Fatal("default realtime capability should be cleared after Close")
	}
	if capability.Topic() != "" {
		t.Fatalf("topic after Close: want empty, got %q", capability.Topic())
	}
	if err := capability.Broadcast(context.Background(), corerealtime.NewEvent("notify.closed", nil)); !errors.Is(err, realtimecap.ErrNotConfigured) {
		t.Fatalf("Broadcast after Close: want ErrNotConfigured, got %v", err)
	}
	if err := realtimecap.Broadcast(context.Background(), "notify.closed", nil); !errors.Is(err, realtimecap.ErrNotConfigured) {
		t.Fatalf("package Broadcast after Close: want ErrNotConfigured, got %v", err)
	}
	if err := capability.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := bus.Publish(context.Background(), testEvent(t, "rt:events")); err != nil {
		t.Fatalf("external eventbus should not be closed by realtime capability: %v", err)
	}
}

func subscribeRealtime(t *testing.T, bus eventbus.Bus, topic string) <-chan *eventbus.Event {
	t.Helper()
	received := make(chan *eventbus.Event, 1)
	subscription, err := bus.Subscribe(context.Background(), topic, func(_ context.Context, event *eventbus.Event) error {
		received <- event
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	t.Cleanup(func() { _ = subscription.Close() })
	return received
}

func receiveEvent(t *testing.T, received <-chan *eventbus.Event) *eventbus.Event {
	t.Helper()
	select {
	case event := <-received:
		return event
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for realtime event")
		return nil
	}
}

func decodeRealtimeEvent(t *testing.T, event *eventbus.Event) corerealtime.Event {
	t.Helper()
	var msg corerealtime.Event
	if event == nil {
		t.Fatal("nil event")
	}
	if err := json.Unmarshal(event.Payload, &msg); err != nil {
		t.Fatalf("Unmarshal realtime event: %v", err)
	}
	return msg
}

func testEvent(t *testing.T, topic string) *eventbus.Event {
	t.Helper()
	event, err := eventbus.NewEvent(context.Background(), topic, nil, nil)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	return event
}

func resetDefaults(t *testing.T) {
	t.Helper()
	if err := realtimecap.CloseDefault(); err != nil {
		t.Fatalf("CloseDefault realtime: %v", err)
	}
	if err := eventbus.CloseDefault(); err != nil {
		t.Fatalf("CloseDefault eventbus: %v", err)
	}
	t.Cleanup(func() {
		_ = realtimecap.CloseDefault()
		_ = eventbus.CloseDefault()
	})
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

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
