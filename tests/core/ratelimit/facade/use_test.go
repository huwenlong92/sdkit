package facade_test

import (
	"context"
	"testing"

	coreratelimit "github.com/huwenlong92/sdkit/core/ratelimit"
	ratelimit "github.com/huwenlong92/sdkit/core/ratelimit/facade"
	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestUseDefaultsToInternal(t *testing.T) {
	capability := ratelimit.Use()
	if !capability.Metadata().Internal {
		t.Fatal("Use() should default to internal capability")
	}

	capability = ratelimit.Use(ratelimit.WithExternal())
	if capability.Metadata().Internal {
		t.Fatal("WithExternal() should expose capability")
	}
}

func TestUseWithoutRedisBindsMemoryStore(t *testing.T) {
	t.Cleanup(func() {
		coreratelimit.SetStore(nil)
	})

	app := runtime.New()
	if err := app.Use(ratelimit.Use()); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := ratelimit.From(app); got == nil {
		t.Fatal("ratelimit.From(app) = nil, want store")
	}
	if coreratelimit.CustomStore == nil {
		t.Fatal("middleware store = nil, want store")
	}
}

func TestUseWithStoreBindsInjectedStore(t *testing.T) {
	t.Cleanup(func() {
		coreratelimit.SetStore(nil)
	})

	store := ratelimit.NewMemoryStore()
	t.Cleanup(func() {
		_ = store.Close()
	})
	app := runtime.New()
	if err := app.Use(ratelimit.Use(ratelimit.WithStore(store))); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := ratelimit.From(app); got != store {
		t.Fatalf("ratelimit.From(app) = %p, want %p", got, store)
	}
}
