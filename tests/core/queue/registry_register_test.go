package queue_test

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/queue"
)

func TestRegistryRegisterSkipsNilMiddleware(t *testing.T) {
	runner := &runtimeRunner{}
	registry := queue.NewRegistry(runner)
	var nilMiddleware queue.Middleware
	called := false

	err := registry.Register("example:task", nilMiddleware, func(context.Context, *queue.Message) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.Runtime().Dispatch(context.Background(), "example:task", nil); err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
	entries := registry.Entries()
	if len(entries) != 1 || entries[0].MiddlewareCount != 0 {
		t.Fatalf("entries = %#v, want one entry without middleware", entries)
	}
}

func TestRegistryRegisterAllRegistersEntries(t *testing.T) {
	runner := &runtimeRunner{}
	registry := queue.NewRegistry(runner)

	err := registry.RegisterAll(
		queue.Register("example:one", func(context.Context, *queue.Message) error {
			return nil
		}),
		queue.Register("example:two", func(context.Context, *queue.Message) error {
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("RegisterAll() error = %v", err)
	}
	if runner.handlers["example:one"] == nil || runner.handlers["example:two"] == nil {
		t.Fatalf("handlers = %#v, want both registrations", runner.handlers)
	}
}
