package tests

import (
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/realtime"
	"github.com/huwenlong92/sdkit/pkg/realtime/gateway"
)

func TestActionContractsAreRuntimeComposable(t *testing.T) {
	var _ realtime.ActionCodec = (*contractActionCodec)(nil)
	var _ realtime.Router = gateway.NewRouter()

	called := false
	handler := realtime.ActionHandlerFunc(func(action *realtime.ActionContext) error {
		called = true
		if action.Client == nil || action.Client.ID != "client-1" {
			t.Fatalf("client: %+v", action.Client)
		}
		if action.Event.Action != "ping" || action.Event.RequestID != "req-1" {
			t.Fatalf("event: %+v", action.Event)
		}
		return nil
	})
	router := gateway.NewRouter()
	router.On("ping", handler)
	runtime := gateway.NewRuntime(gateway.WithRouter(router))

	if err := runtime.Handle(&realtime.ActionContext{
		Client: &realtime.Client{ID: "client-1"},
		Event:  (&realtime.Event{Action: "ping", RequestID: "req-1"}).Normalize(),
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestNilActionHandlerReturnsError(t *testing.T) {
	handler := gateway.BuildPipeline(&realtime.Route{Action: "ping", Handlers: []realtime.HandlerFunc{nil}})
	if err := handler(&realtime.ActionContext{}); !errors.Is(err, realtime.ErrNilActionHandler) {
		t.Fatalf("handler: want ErrNilActionHandler, got %v", err)
	}
}

func TestActionErrorWrapsCause(t *testing.T) {
	err := realtime.NewActionError("missing_action", "missing websocket action", realtime.ErrEmptyAction)
	if err.Code != "missing_action" || err.Message != "missing websocket action" {
		t.Fatalf("action error fields: %+v", err)
	}
	if !errors.Is(err, realtime.ErrEmptyAction) {
		t.Fatalf("ActionError should wrap ErrEmptyAction: %v", err)
	}
	if got := err.Error(); got != "missing_action: missing websocket action" {
		t.Fatalf("Error: got %q", got)
	}
}

type contractActionCodec struct{}

func (*contractActionCodec) DecodeAction(payload []byte) (realtime.ActionMessage, error) {
	return realtime.ActionMessage{Action: "ping", RequestID: "req-1", Payload: payload}, nil
}

func (*contractActionCodec) EncodeAction(message realtime.ActionMessage) ([]byte, error) {
	return message.Payload, nil
}
