package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/huwenlong92/sdkit/core/eventbus"
	"github.com/huwenlong92/sdkit/core/realtime"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"
	eventbusmemory "github.com/huwenlong92/sdkit/pkg/eventbus/memory"
	eventbuspublisher "github.com/huwenlong92/sdkit/pkg/realtime/publisher/eventbus"
)

func TestPublisherDoesNotWriteTrackIDAsRealtimeTraceID(t *testing.T) {
	bus := eventbusmemory.New()
	got := make(chan *eventbus.Event, 1)

	_, err := bus.Subscribe(context.Background(), realtime.DefaultTopic, func(_ context.Context, event *eventbus.Event) error {
		got <- event
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	ctx := tracking.WithTrackID(context.Background(), "track-realtime")
	ctx = requestid.WithRequestID(ctx, "request-realtime")
	publisher := eventbuspublisher.New(bus, realtime.DefaultTopic)
	if err := publisher.PushUser(ctx, "1001", realtime.NewEvent("notify", map[string]string{"title": "hello"})); err != nil {
		t.Fatalf("PushUser: %v", err)
	}

	event := <-got
	if got := tracing.HeaderValue(event.Headers, tracking.Header); got != "track-realtime" {
		t.Fatalf("track header: want track-realtime, got %q", got)
	}
	if got := tracing.HeaderValue(event.Headers, requestid.Header); got != "request-realtime" {
		t.Fatalf("request header: want request-realtime, got %q", got)
	}

	var msg realtime.Event
	if err := json.Unmarshal(event.Payload, &msg); err != nil {
		t.Fatalf("Unmarshal realtime event: %v", err)
	}
	if msg.TraceID != "" {
		t.Fatalf("realtime trace_id should not reuse track/request id, got %q", msg.TraceID)
	}
	if msg.Target == nil || msg.Target.Type != realtime.TargetUser || msg.Target.UserID != "1001" {
		t.Fatalf("target: want user/1001, got %+v", msg.Target)
	}
	if len(msg.Payload) == 0 {
		t.Fatalf("payload should mirror data, payload=%s data=%v", string(msg.Payload), msg.Data)
	}
	if got := tracing.HeaderValue(msg.Headers, tracking.Header); got != "track-realtime" {
		t.Fatalf("realtime headers track_id: want track-realtime, got %q", got)
	}
	if got := tracing.HeaderValue(msg.Headers, requestid.Header); got != "request-realtime" {
		t.Fatalf("realtime headers request_id: want request-realtime, got %q", got)
	}
}

func TestPublisherPushRoomWritesRoomID(t *testing.T) {
	bus := eventbusmemory.New()
	got := make(chan *eventbus.Event, 1)

	_, err := bus.Subscribe(context.Background(), realtime.DefaultTopic, func(_ context.Context, event *eventbus.Event) error {
		got <- event
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	publisher := eventbuspublisher.New(bus, realtime.DefaultTopic)
	if err := publisher.PushRoom(context.Background(), "room-1", realtime.NewEvent("notify", map[string]string{"title": "hello"})); err != nil {
		t.Fatalf("PushRoom: %v", err)
	}

	event := <-got
	var msg realtime.Event
	if err := json.Unmarshal(event.Payload, &msg); err != nil {
		t.Fatalf("Unmarshal realtime event: %v", err)
	}
	if msg.RoomID != "room-1" {
		t.Fatalf("room_id: want room-1, got %q", msg.RoomID)
	}
	if msg.Target == nil || msg.Target.Type != realtime.TargetRoom || msg.Target.RoomID != "room-1" {
		t.Fatalf("target: want room/room-1, got %+v", msg.Target)
	}
}

func TestPublisherEventProtocolFields(t *testing.T) {
	bus := eventbusmemory.New()
	got := make(chan *eventbus.Event, 1)

	_, err := bus.Subscribe(context.Background(), realtime.DefaultTopic, func(_ context.Context, event *eventbus.Event) error {
		got <- event
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	ctx := tracking.WithTrackID(context.Background(), "track-protocol")
	ctx = requestid.WithRequestID(ctx, "request-protocol")
	publisher := eventbuspublisher.New(bus, realtime.DefaultTopic)
	if err := publisher.Broadcast(ctx, realtime.NewEvent("im.send_message", map[string]string{"message_id": "m-1"})); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}

	event := <-got
	if got := tracing.HeaderValue(event.Headers, tracking.Header); got != "track-protocol" {
		t.Fatalf("eventbus track header: want track-protocol, got %q", got)
	}
	if got := tracing.HeaderValue(event.Headers, requestid.Header); got != "request-protocol" {
		t.Fatalf("eventbus request header: want request-protocol, got %q", got)
	}

	var msg realtime.Event
	if err := json.Unmarshal(event.Payload, &msg); err != nil {
		t.Fatalf("Unmarshal realtime event: %v", err)
	}
	if msg.Target == nil || msg.Target.Type != realtime.TargetBroadcast {
		t.Fatalf("target: want broadcast, got %+v", msg.Target)
	}
	if len(msg.Payload) == 0 {
		t.Fatalf("payload should mirror data, payload=%s data=%v", string(msg.Payload), msg.Data)
	}
	if got := tracing.HeaderValue(msg.Headers, tracking.Header); got != "track-protocol" {
		t.Fatalf("realtime track header: want track-protocol, got %q", got)
	}
	if got := tracing.HeaderValue(msg.Headers, requestid.Header); got != "request-protocol" {
		t.Fatalf("realtime request header: want request-protocol, got %q", got)
	}
	if msg.TraceID != "" {
		t.Fatalf("trace_id should not reuse track/request id, got %q", msg.TraceID)
	}
}
