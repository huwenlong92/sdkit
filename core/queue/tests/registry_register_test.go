package tests

import (
	"context"
	"testing"

	corequeue "github.com/huwenlong92/sdkit/core/queue"
)

func TestRegistryRegisterSkipsNilMiddleware(t *testing.T) {
	runner := &runtimeRunner{}
	registry := corequeue.NewRegistry(runner)
	var nilMiddleware corequeue.Middleware
	called := false

	err := registry.Register("example:task", nilMiddleware, func(context.Context, *corequeue.Message) error {
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
	registry := corequeue.NewRegistry(runner)

	err := registry.RegisterAll(
		corequeue.Register("example:one", func(context.Context, *corequeue.Message) error {
			return nil
		}),
		corequeue.Register("example:two", func(context.Context, *corequeue.Message) error {
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
