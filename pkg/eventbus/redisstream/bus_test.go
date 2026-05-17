package redisstream

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/eventbus"
)

func TestCapability(t *testing.T) {
	bus := New(nil, "sdkitgo:rt", "group", "consumer")
	capability := bus.Capability()
	if !capability.Persistent || !capability.ConsumerGrp {
		t.Fatalf("redis stream capability = %+v, want persistent consumer group", capability)
	}
	if capability.Fanout || capability.Delay || capability.Retry {
		t.Fatalf("unexpected redis stream capability: %+v", capability)
	}
}

func TestNilClientErrors(t *testing.T) {
	bus := New(nil, "sdkitgo:rt", "group", "consumer")
	if err := bus.Publish(context.Background(), testEvent(t, "rt:events")); err == nil || err.Error() != "eventbus redis stream client is nil" {
		t.Fatalf("Publish error = %v, want nil client error", err)
	}
	_, err := bus.Subscribe(context.Background(), "rt:events", func(context.Context, *eventbus.Event) error { return nil })
	if err == nil || err.Error() != "eventbus redis stream client is nil" {
		t.Fatalf("Subscribe error = %v, want nil client error", err)
	}
}

func TestCloseRejectsPublishAndSubscribe(t *testing.T) {
	bus := New(nil, "", "group", "consumer")
	_ = bus.Close()
	if err := bus.Publish(context.Background(), testEvent(t, "rt:events")); !errors.Is(err, ErrClosed) {
		t.Fatalf("Publish error = %v, want ErrClosed", err)
	}
	if _, err := bus.Subscribe(context.Background(), "rt:events", func(context.Context, *eventbus.Event) error { return nil }); !errors.Is(err, ErrClosed) {
		t.Fatalf("Subscribe error = %v, want ErrClosed", err)
	}
}

func testEvent(t *testing.T, topic string) *eventbus.Event {
	t.Helper()
	event, err := eventbus.NewEvent(context.Background(), topic, []byte("x"), nil)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	return event
}

func TestSanitize(t *testing.T) {
	if got := sanitize(" node:one "); got != "node_one" {
		t.Fatalf("sanitize = %q", got)
	}
	if got := sanitize(""); got != "default" {
		t.Fatalf("sanitize empty = %q", got)
	}
}

func TestTopicPrefix(t *testing.T) {
	bus := New(nil, "sdkitgo:rt", "group", "consumer")
	if got := bus.topic("rt:events"); got != "sdkitgo:rt:rt:events" {
		t.Fatalf("topic = %q", got)
	}
}
