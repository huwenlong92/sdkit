package tests

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/realtime"
	"github.com/huwenlong92/sdkit/pkg/realtime/transport"
)

func TestJSONActionCodecDecodesDataPayload(t *testing.T) {
	codec := transport.NewJSONActionCodec()
	message, err := codec.DecodeAction([]byte(`{"action":"room.join","request_id":"req-1","headers":{"x":"y"},"data":{"room_id":"room-1"}}`))
	if err != nil {
		t.Fatalf("DecodeAction: %v", err)
	}
	if message.Action != "room.join" || message.RequestID != "req-1" {
		t.Fatalf("message: %+v", message)
	}
	if message.Headers["x"] != "y" {
		t.Fatalf("headers: %+v", message.Headers)
	}
	var data map[string]string
	if err := json.Unmarshal(message.Payload, &data); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if data["room_id"] != "room-1" {
		t.Fatalf("payload data: %+v", data)
	}
}

func TestJSONActionCodecDecodesPayloadField(t *testing.T) {
	codec := transport.NewJSONActionCodec()
	message, err := codec.DecodeAction([]byte(`{"action":"device.bind","payload":{"device_id":"device-1"}}`))
	if err != nil {
		t.Fatalf("DecodeAction: %v", err)
	}
	var data map[string]string
	if err := json.Unmarshal(message.Payload, &data); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if data["device_id"] != "device-1" {
		t.Fatalf("payload data: %+v", data)
	}
}

func TestJSONActionCodecEncodesMessage(t *testing.T) {
	codec := transport.NewJSONActionCodec()
	payload, err := codec.EncodeAction(realtime.ActionMessage{
		Action:    "ping",
		RequestID: "req-1",
		Headers:   map[string]string{"trace": "t1"},
		Payload:   []byte(`{"ok":true}`),
	})
	if err != nil {
		t.Fatalf("EncodeAction: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode encoded payload: %v", err)
	}
	if got["action"] != "ping" || got["request_id"] != "req-1" {
		t.Fatalf("encoded payload: %+v", got)
	}
	data, ok := got["data"].(map[string]any)
	if !ok || data["ok"] != true {
		t.Fatalf("encoded data: %+v", got["data"])
	}
}

func TestJSONActionCodecRejectsInvalidEncodedPayload(t *testing.T) {
	codec := transport.NewJSONActionCodec()
	_, err := codec.EncodeAction(realtime.ActionMessage{Action: "ping", Payload: []byte("not-json")})
	if !errors.Is(err, realtime.ErrInvalidAction) {
		t.Fatalf("EncodeAction: want ErrInvalidAction, got %v", err)
	}
}
