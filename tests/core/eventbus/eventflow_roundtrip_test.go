package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/eventbus"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"
	eventbusmemory "github.com/huwenlong92/sdkit/pkg/eventbus/memory"
)

func TestEventFlowCorrelationRoundTripThroughMemoryBus(t *testing.T) {
	restore := installTracing(t)
	defer restore()

	bus := eventbusmemory.New()
	defer func() {
		if err := bus.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

	type realtimePayload struct {
		Module string          `json:"module"`
		Event  string          `json:"event"`
		Data   json.RawMessage `json:"data"`
	}

	seen := make(chan struct {
		ctx   context.Context
		event *eventbus.Event
	}, 1)
	if _, err := bus.Subscribe(context.Background(), "rt:events", func(ctx context.Context, event *eventbus.Event) error {
		seen <- struct {
			ctx   context.Context
			event *eventbus.Event
		}{ctx: ctx, event: event}
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	ctx := tracking.WithTrackID(context.Background(), "track-flow")
	ctx = requestid.WithRequestID(ctx, "request-flow")
	ctx, span := tracing.StartSpan(ctx, "eventflow.publish")
	defer span.End()
	wantTraceID := tracing.TraceID(ctx)

	payload := realtimePayload{
		Module: "im",
		Event:  "send_message",
		Data:   json.RawMessage(`{"message_id":"m-1"}`),
	}
	published, err := eventbus.NewJSONEvent(ctx, "rt:events", payload, map[string]string{
		eventbus.HeaderConnectionID: "conn-flow",
		eventbus.HeaderSessionID:    "session-flow",
	})
	if err != nil {
		t.Fatalf("NewJSONEvent: %v", err)
	}
	if err := bus.Publish(ctx, published); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	var got struct {
		ctx   context.Context
		event *eventbus.Event
	}
	select {
	case got = <-seen:
	case <-time.After(time.Second):
		t.Fatal("handler was not called")
	}

	if got.event.Topic != "rt:events" {
		t.Fatalf("event topic: want rt:events, got %q", got.event.Topic)
	}
	if gotHeader := tracing.HeaderValue(got.event.Headers, eventbus.HeaderTraceparent); gotHeader == "" {
		t.Fatal("traceparent header should not be empty")
	}
	if gotHeader := tracing.TraceIDFromHeaders(got.event.Headers); gotHeader != wantTraceID {
		t.Fatalf("trace_id from headers: want %s, got %s", wantTraceID, gotHeader)
	}
	if gotHeader := tracing.HeaderValue(got.event.Headers, tracking.Header); gotHeader != "track-flow" {
		t.Fatalf("track header: want track-flow, got %q", gotHeader)
	}
	if gotHeader := tracing.HeaderValue(got.event.Headers, requestid.Header); gotHeader != "request-flow" {
		t.Fatalf("request header: want request-flow, got %q", gotHeader)
	}
	if gotHeader := got.event.Headers[eventbus.HeaderConnectionID]; gotHeader != "conn-flow" {
		t.Fatalf("connection_id header: want conn-flow, got %q", gotHeader)
	}
	if gotHeader := got.event.Headers[eventbus.HeaderSessionID]; gotHeader != "session-flow" {
		t.Fatalf("session_id header: want session-flow, got %q", gotHeader)
	}

	if gotTraceID := tracing.TraceID(got.ctx); gotTraceID != wantTraceID {
		t.Fatalf("handler trace_id: want %s, got %s", wantTraceID, gotTraceID)
	}
	if gotTrackID := tracking.TrackID(got.ctx); gotTrackID != "track-flow" {
		t.Fatalf("handler track_id: want track-flow, got %q", gotTrackID)
	}
	if gotRequestID := tracing.RequestID(got.ctx); gotRequestID != "request-flow" {
		t.Fatalf("handler request_id: want request-flow, got %q", gotRequestID)
	}

	var decoded realtimePayload
	if err := (eventbus.JSONCodec{}).Unmarshal(got.event.Payload, &decoded); err != nil {
		t.Fatalf("unmarshal realtime payload: %v", err)
	}
	if decoded.Module != payload.Module || decoded.Event != payload.Event {
		t.Fatalf("payload route: want %s/%s, got %s/%s", payload.Module, payload.Event, decoded.Module, decoded.Event)
	}
}
