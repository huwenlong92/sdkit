package memory_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/eventbus"
	"github.com/huwenlong92/sdkit/pkg/eventbus/memory"
)

func TestPublishSubscribe(t *testing.T) {
	bus := memory.New()
	ctx := context.Background()
	got := make(chan string, 1)

	if _, err := bus.Subscribe(ctx, "rt:events", func(_ context.Context, event *eventbus.Event) error {
		got <- event.Topic + ":" + string(event.Payload)
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if err := bus.Publish(ctx, testEvent(t, ctx, "rt:events", []byte("hello"), nil)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if msg := <-got; msg != "rt:events:hello" {
		t.Fatalf("message: want rt:events:hello, got %s", msg)
	}
}

func TestPublishMultipleSubscribers(t *testing.T) {
	bus := memory.New()
	ctx := context.Background()
	var count int64

	for i := 0; i < 2; i++ {
		if _, err := bus.Subscribe(ctx, "rt:events", func(context.Context, *eventbus.Event) error {
			atomic.AddInt64(&count, 1)
			return nil
		}); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	}

	if err := bus.Publish(ctx, testEvent(t, ctx, "rt:events", []byte("hello"), nil)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if count != 2 {
		t.Fatalf("subscriber calls: want 2, got %d", count)
	}
}

func TestCloseRejectsPublishAndSubscribe(t *testing.T) {
	bus := memory.New()
	_ = bus.Close()

	if err := bus.Publish(context.Background(), testEvent(t, context.Background(), "rt:events", []byte("x"), nil)); !errors.Is(err, memory.ErrClosed) {
		t.Fatalf("Publish error: want ErrClosed, got %v", err)
	}
	if _, err := bus.Subscribe(context.Background(), "rt:events", func(context.Context, *eventbus.Event) error { return nil }); !errors.Is(err, memory.ErrClosed) {
		t.Fatalf("Subscribe error: want ErrClosed, got %v", err)
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	bus := memory.New()
	ctx := context.Background()
	var count int64

	subscription, err := bus.Subscribe(ctx, "rt:events", func(context.Context, *eventbus.Event) error {
		atomic.AddInt64(&count, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := bus.Publish(ctx, testEvent(t, ctx, "rt:events", []byte("first"), nil)); err != nil {
		t.Fatalf("Publish first: %v", err)
	}
	if err := subscription.Close(); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if err := bus.Publish(ctx, testEvent(t, ctx, "rt:events", []byte("second"), nil)); err != nil {
		t.Fatalf("Publish second: %v", err)
	}
	if count != 1 {
		t.Fatalf("subscriber calls: want 1, got %d", count)
	}
}

func TestSubscribeContextCancelStopsDelivery(t *testing.T) {
	bus := memory.New()
	ctx, cancel := context.WithCancel(context.Background())
	var count int64

	if _, err := bus.Subscribe(ctx, "rt:events", func(context.Context, *eventbus.Event) error {
		atomic.AddInt64(&count, 1)
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()
	time.Sleep(10 * time.Millisecond)
	if err := bus.Publish(context.Background(), testEvent(t, context.Background(), "rt:events", []byte("x"), nil)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if count != 0 {
		t.Fatalf("subscriber calls after cancel: want 0, got %d", count)
	}
}

func TestHandlerPanicRecovered(t *testing.T) {
	bus := memory.New()
	ctx := context.Background()

	if _, err := bus.Subscribe(ctx, "rt:events", func(context.Context, *eventbus.Event) error {
		panic("boom")
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := bus.Publish(ctx, testEvent(t, ctx, "rt:events", []byte("x"), nil)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
}

func TestPublishIncludesTraceHeader(t *testing.T) {
	bus := memory.New()
	ctx := context.Background()
	got := make(chan string, 1)

	if _, err := bus.Subscribe(ctx, "rt:events", func(_ context.Context, event *eventbus.Event) error {
		got <- event.Headers[eventbus.HeaderTraceID]
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := bus.Publish(ctx, testEvent(t, ctx, "rt:events", []byte("x"), map[string]string{eventbus.HeaderTraceID: "trace-1"})); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if traceID := <-got; traceID != "trace-1" {
		t.Fatalf("trace id: want trace-1, got %s", traceID)
	}
}

func TestConcurrentPublish(t *testing.T) {
	bus := memory.New()
	ctx := context.Background()
	var count int64

	if _, err := bus.Subscribe(ctx, "rt:events", func(context.Context, *eventbus.Event) error {
		atomic.AddInt64(&count, 1)
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := bus.Publish(ctx, testEvent(t, ctx, "rt:events", []byte("x"), nil)); err != nil {
				t.Errorf("Publish: %v", err)
			}
		}()
	}
	wg.Wait()

	if count != 64 {
		t.Fatalf("subscriber calls: want 64, got %d", count)
	}
}

func testEvent(t *testing.T, ctx context.Context, topic string, payload []byte, headers map[string]string) *eventbus.Event {
	t.Helper()
	event, err := eventbus.NewEvent(ctx, topic, payload, headers)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	return event
}
