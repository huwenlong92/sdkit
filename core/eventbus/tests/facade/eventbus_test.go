package facade_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	coreeventbus "github.com/huwenlong92/sdkit/core/eventbus"
	eventbuscap "github.com/huwenlong92/sdkit/core/eventbus/facade"
	coreruntime "github.com/huwenlong92/sdkit/core/runtime"
	eventbusmemory "github.com/huwenlong92/sdkit/pkg/eventbus/memory"

	goredis "github.com/redis/go-redis/v9"
)

func TestNewMemorySetsDefaultAndRoundTrips(t *testing.T) {
	resetDefault(t)

	capability, err := eventbuscap.New(eventbuscap.Config{Driver: eventbuscap.DriverMemory})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if capability.Driver() != eventbuscap.DriverMemory {
		t.Fatalf("driver: %s", capability.Driver())
	}
	if got := coreeventbus.Default(); got != capability.Bus() {
		t.Fatal("default bus was not set to capability bus")
	}

	received := make(chan *coreeventbus.Event, 1)
	subscription, err := capability.Subscribe(context.Background(), "demo.topic", func(_ context.Context, event *coreeventbus.Event) error {
		received <- event
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = subscription.Close() }()

	if err := capability.Publish(context.Background(), testEvent(t, "demo.topic")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case event := <-received:
		if event.Topic != "demo.topic" {
			t.Fatalf("event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for published event")
	}

	if err := capability.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if coreeventbus.Default() != nil {
		t.Fatal("default bus should be cleared after owned default close")
	}
	if err := capability.Publish(context.Background(), testEvent(t, "demo.topic")); !errors.Is(err, eventbuscap.ErrNotConfigured) {
		t.Fatalf("Publish after close: want ErrNotConfigured, got %v", err)
	}
}

func TestWithoutDefaultStillClosesOwnedBus(t *testing.T) {
	resetDefault(t)

	capability, err := eventbuscap.New(eventbuscap.Config{Driver: eventbuscap.DriverMemory}, eventbuscap.WithoutDefault())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	bus := capability.Bus()
	if bus == nil {
		t.Fatal("bus should be configured")
	}
	if coreeventbus.Default() != nil {
		t.Fatal("default bus should not be set")
	}

	if err := capability.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := bus.Publish(context.Background(), testEvent(t, "demo.topic")); !errors.Is(err, coreeventbus.ErrClosed) {
		t.Fatalf("owned bus after close: want ErrClosed, got %v", err)
	}
	if err := capability.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestReusesExistingDefaultWithoutOwning(t *testing.T) {
	resetDefault(t)
	bus := eventbusmemory.New()
	coreeventbus.SetDefaultWithDriver(bus, eventbuscap.DriverMemory)

	capability, err := eventbuscap.New(eventbuscap.Config{Driver: eventbuscap.DriverMemory})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if capability.Bus() != bus {
		t.Fatal("capability should reuse existing default bus")
	}
	if err := capability.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if coreeventbus.Default() != bus {
		t.Fatal("reused default bus should not be cleared")
	}
	if err := bus.Publish(context.Background(), testEvent(t, "demo.topic")); err != nil {
		t.Fatalf("reused default should remain open: %v", err)
	}
}

func TestInjectedBusCloseOwnership(t *testing.T) {
	t.Run("unowned clears default but keeps bus open", func(t *testing.T) {
		resetDefault(t)
		bus := eventbusmemory.New()
		capability, err := eventbuscap.New(eventbuscap.Config{Driver: eventbuscap.DriverMemory}, eventbuscap.WithBus(bus, eventbuscap.DriverMemory))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if coreeventbus.Default() != bus {
			t.Fatal("injected bus should be registered as default")
		}
		if err := capability.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if coreeventbus.Default() != nil {
			t.Fatal("default bus should be cleared")
		}
		if err := bus.Publish(context.Background(), testEvent(t, "demo.topic")); err != nil {
			t.Fatalf("unowned injected bus should stay open: %v", err)
		}
		_ = bus.Close()
	})

	t.Run("owned closes bus and clears default", func(t *testing.T) {
		resetDefault(t)
		bus := eventbusmemory.New()
		capability, err := eventbuscap.New(eventbuscap.Config{Driver: eventbuscap.DriverMemory}, eventbuscap.WithOwnedBus(bus, eventbuscap.DriverMemory))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := capability.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if coreeventbus.Default() != nil {
			t.Fatal("default bus should be cleared")
		}
		if err := bus.Publish(context.Background(), testEvent(t, "demo.topic")); !errors.Is(err, coreeventbus.ErrClosed) {
			t.Fatalf("owned injected bus after close: want ErrClosed, got %v", err)
		}
	})
}

func TestRedisDriversRequireExternalClient(t *testing.T) {
	resetDefault(t)

	for _, driver := range []string{eventbuscap.DriverRedis, eventbuscap.DriverRedisStream} {
		t.Run(driver, func(t *testing.T) {
			_, err := eventbuscap.New(eventbuscap.Config{Driver: driver})
			if !errors.Is(err, eventbuscap.ErrRedisClientRequired) {
				t.Fatalf("New: want ErrRedisClientRequired, got %v", err)
			}
		})
	}
}

func TestRedisDriversUseProvidedClient(t *testing.T) {
	resetDefault(t)
	client := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = client.Close() })

	for _, driver := range []string{eventbuscap.DriverRedis, eventbuscap.DriverRedisStream} {
		t.Run(driver, func(t *testing.T) {
			capability, err := eventbuscap.New(eventbuscap.Config{
				Driver:      driver,
				TopicPrefix: "test:",
				NodeName:    "test-node",
			}, eventbuscap.WithRedisClient(client), eventbuscap.WithoutDefault())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if capability.Driver() != driver {
				t.Fatalf("driver: want %s, got %s", driver, capability.Driver())
			}
			if capability.Bus() == nil {
				t.Fatal("bus should be configured")
			}
			if coreeventbus.Default() != nil {
				t.Fatal("default bus should not be set")
			}
			if err := capability.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
		})
	}
}

func TestNATSDriverRequiresAddr(t *testing.T) {
	resetDefault(t)

	_, err := eventbuscap.New(eventbuscap.Config{Driver: eventbuscap.DriverNATS})
	if err == nil || !strings.Contains(err.Error(), "nats addr") {
		t.Fatalf("New: want nats addr error, got %v", err)
	}
}

func TestDriverErrors(t *testing.T) {
	t.Run("invalid driver", func(t *testing.T) {
		resetDefault(t)
		_, err := eventbuscap.New(eventbuscap.Config{Driver: "bad"})
		if err == nil || !strings.Contains(err.Error(), "invalid") {
			t.Fatalf("New: want invalid driver error, got %v", err)
		}
	})

	t.Run("default mismatch", func(t *testing.T) {
		resetDefault(t)
		coreeventbus.SetDefaultWithDriver(eventbusmemory.New(), eventbuscap.DriverMemory)
		client := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:0"})
		t.Cleanup(func() { _ = client.Close() })
		_, err := eventbuscap.New(eventbuscap.Config{Driver: eventbuscap.DriverRedis}, eventbuscap.WithRedisClient(client))
		if err == nil || !strings.Contains(err.Error(), "mismatch") {
			t.Fatalf("New: want mismatch error, got %v", err)
		}
	})
}

func TestRuntimeUseBindsEventBusCapability(t *testing.T) {
	resetDefault(t)
	app := coreruntime.New()
	if err := app.Use(eventbuscap.Use(eventbuscap.WithConfig(eventbuscap.Config{Driver: eventbuscap.DriverMemory}))); err != nil {
		t.Fatalf("Use: %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	service := eventbuscap.From(app)
	if service == nil {
		t.Fatal("eventbus From(app) = nil")
	}
	if service.Driver() != eventbuscap.DriverMemory {
		t.Fatalf("driver = %q, want memory", service.Driver())
	}
	if got := coreeventbus.Default(); got != service.Bus() {
		t.Fatal("runtime eventbus capability should set default bus")
	}
	if err := service.Publish(context.Background(), testEvent(t, "demo.topic")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if coreeventbus.Default() != nil {
		t.Fatal("runtime stop should clear default eventbus")
	}
}

func testEvent(t *testing.T, topic string) *coreeventbus.Event {
	t.Helper()
	event, err := coreeventbus.NewJSONEvent(context.Background(), topic, map[string]any{"ok": true}, nil)
	if err != nil {
		t.Fatalf("NewJSONEvent: %v", err)
	}
	return event
}

func resetDefault(t *testing.T) {
	t.Helper()
	if err := coreeventbus.CloseDefault(); err != nil {
		t.Fatalf("CloseDefault: %v", err)
	}
	t.Cleanup(func() { _ = coreeventbus.CloseDefault() })
}
