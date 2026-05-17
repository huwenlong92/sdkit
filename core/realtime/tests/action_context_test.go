package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/realtime"
)

func TestActionContextShouldBindJSONUsesMessagePayload(t *testing.T) {
	action := &realtime.Context{
		Message: realtime.ActionMessage{Payload: []byte(`{"room_id":"room-1"}`)},
		Raw:     []byte(`{"action":"room.join","data":{"room_id":"raw-room"}}`),
	}

	var req struct {
		RoomID string `json:"room_id"`
	}
	if err := action.ShouldBindJSON(&req); err != nil {
		t.Fatalf("ShouldBindJSON: %v", err)
	}
	if req.RoomID != "room-1" {
		t.Fatalf("room_id: want room-1, got %q", req.RoomID)
	}
}

func TestActionContextShouldBindJSONTreatsEmptyPayloadAsObject(t *testing.T) {
	action := &realtime.Context{}

	var req struct {
		RoomID string `json:"room_id"`
	}
	if err := action.ShouldBindJSON(&req); err != nil {
		t.Fatalf("ShouldBindJSON empty payload: %v", err)
	}
	if req.RoomID != "" {
		t.Fatalf("room_id should stay empty, got %q", req.RoomID)
	}
}

func TestActionContextReplyUsesGateway(t *testing.T) {
	gateway := &captureGateway{}
	action := &realtime.Context{
		Client:  &realtime.Client{ID: "client-1"},
		Gateway: gateway,
		Message: realtime.ActionMessage{RequestID: "req-1"},
	}
	action.SetContext(context.Background())

	if err := action.Reply("pong", map[string]any{"ok": true}); err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if gateway.clientID != "client-1" {
		t.Fatalf("client id: want client-1, got %q", gateway.clientID)
	}
	if gateway.event == nil || gateway.event.Action != "pong" || gateway.event.RequestID != "req-1" {
		t.Fatalf("event: %+v", gateway.event)
	}
}

func TestActionContextPushesUseGateway(t *testing.T) {
	gateway := &captureGateway{}
	action := &realtime.Context{Gateway: gateway}

	if err := action.PushUser("user-1", realtime.NewEvent("notify", nil)); err != nil {
		t.Fatalf("PushUser: %v", err)
	}
	if gateway.userID != "user-1" {
		t.Fatalf("user id: want user-1, got %q", gateway.userID)
	}
	if err := action.PushRoom("room-1", realtime.NewEvent("room.notify", nil)); err != nil {
		t.Fatalf("PushRoom: %v", err)
	}
	if gateway.roomID != "room-1" {
		t.Fatalf("room id: want room-1, got %q", gateway.roomID)
	}
}

func TestActionContextActionErrorWrapsCause(t *testing.T) {
	cause := errors.New("cause")
	err := (&realtime.Context{}).ActionError("invalid_payload", "invalid payload", cause)

	var actionErr *realtime.ActionError
	if !errors.As(err, &actionErr) {
		t.Fatalf("ActionError type: %T", err)
	}
	if actionErr.Code != "invalid_payload" || actionErr.Message != "invalid payload" {
		t.Fatalf("ActionError fields: %+v", actionErr)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("ActionError should wrap cause: %v", err)
	}
}

type captureGateway struct {
	event    *realtime.Event
	clientID string
	userID   string
	roomID   string
}

func (g *captureGateway) Handle(*realtime.ActionContext) error {
	return nil
}

func (g *captureGateway) Publish(context.Context, *realtime.Event) error {
	return nil
}

func (g *captureGateway) PushUser(_ context.Context, userID string, event *realtime.Event) error {
	g.userID = userID
	g.event = event
	return nil
}

func (g *captureGateway) PushClient(_ context.Context, clientID string, event *realtime.Event) error {
	g.clientID = clientID
	g.event = event
	return nil
}

func (g *captureGateway) PushRoom(_ context.Context, roomID string, event *realtime.Event) error {
	g.roomID = roomID
	g.event = event
	return nil
}

func (g *captureGateway) Broadcast(_ context.Context, event *realtime.Event) error {
	g.event = event
	return nil
}
