package producer

import (
	"context"
	"reflect"
	"testing"

	corequeue "github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/runtime"
)

type testProducer struct {
	closed bool
}

func (p *testProducer) Enqueue(context.Context, corequeue.Task, ...corequeue.Option) (*corequeue.TaskInfo, error) {
	return &corequeue.TaskInfo{ID: "test-task"}, nil
}

func (p *testProducer) BatchEnqueue(context.Context, []corequeue.Task, ...corequeue.Option) ([]*corequeue.TaskInfo, error) {
	return []*corequeue.TaskInfo{{ID: "test-task"}}, nil
}

func (p *testProducer) Close() error {
	p.closed = true
	return nil
}

func TestUseRegistersQueueProducerCapability(t *testing.T) {
	producer := &testProducer{}
	capability := Use(WithProducer(producer))

	metadata := capability.Metadata()
	if metadata.Name != string(KeyQueue) {
		t.Fatalf("metadata name = %q, want %q", metadata.Name, KeyQueue)
	}
	if metadata.Description != "Queue producer" {
		t.Fatalf("metadata description = %q, want Queue producer", metadata.Description)
	}
	if metadata.Group != runtime.GroupSystem {
		t.Fatalf("metadata group = %q, want %q", metadata.Group, runtime.GroupSystem)
	}
	if metadata.Scope != runtime.ScopeGlobal {
		t.Fatalf("metadata scope = %q, want %q", metadata.Scope, runtime.ScopeGlobal)
	}

	app := runtime.New()
	if err := app.RegisterCapabilities(capability); err != nil {
		t.Fatalf("RegisterCapabilities() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := From(app); got != producer {
		t.Fatalf("From(app) = %v, want injected producer", got)
	}
	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !producer.closed {
		t.Fatal("producer was not closed")
	}
}

func TestUseServiceLocalNameDoesNotSetDefault(t *testing.T) {
	producer := &testProducer{}
	app := runtime.New()
	if err := app.RegisterCapabilities(Use(WithName("api.queue.producer"), WithProducer(producer))); err != nil {
		t.Fatalf("RegisterCapabilities() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	value, ok := app.Container().Get(runtime.Key("api.queue.producer"))
	if !ok {
		t.Fatal("api.queue.producer was not bound")
	}
	if value != producer {
		t.Fatalf("api.queue.producer = %v, want injected producer", value)
	}
	if got := From(app); got != nil {
		t.Fatalf("From(app) = %v, want nil default producer", got)
	}
	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !producer.closed {
		t.Fatal("producer was not closed")
	}
}

func TestFromServiceContextReadsLocalProducer(t *testing.T) {
	producer := &testProducer{}
	ctx := testServiceContext{values: map[string]any{
		"api.queue.producer": producer,
	}}

	got, ok := FromServiceContext(ctx)
	if !ok || got != producer {
		t.Fatalf("FromServiceContext() = %v, %v; want producer, true", got, ok)
	}
	queueRuntime := RuntimeFromServiceContext(ctx)
	if queueRuntime == nil || queueRuntime.Client() != producer {
		t.Fatalf("RuntimeFromServiceContext() = %v, want runtime with producer", queueRuntime)
	}
}

func TestUseConfigLoaderRunsAfterBootstrap(t *testing.T) {
	app := runtime.New()
	producer := &testProducer{}
	order := make([]string, 0, 2)
	if err := app.RegisterCapabilities(
		Use(
			WithProducer(producer),
			WithConfigLoader(func(app *runtime.App) (Config, error) {
				if _, ok := app.Container().Get(runtime.Key("config")); !ok {
					t.Fatal("config not initialized before queue config loader")
				}
				order = append(order, "queue.loader")
				return Config{}, nil
			}),
		),
		runtime.NewCapability("bootstrap", func(app *runtime.App) error {
			order = append(order, "bootstrap")
			return app.Container().Bind(runtime.Key("config"), true)
		}),
	); err != nil {
		t.Fatalf("RegisterCapabilities() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []string{"bootstrap", "queue.loader"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

type testServiceContext struct {
	values map[string]any
}

func (c testServiceContext) CapabilityLocalFirst(name string) (any, bool) {
	if value, ok := c.values[name]; ok {
		return value, true
	}
	value, ok := c.values["api."+name]
	return value, ok
}
