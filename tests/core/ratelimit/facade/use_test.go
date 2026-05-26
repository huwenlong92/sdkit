package facade_test

import (
	"context"
	"testing"

	ratelimitfacade "github.com/huwenlong92/sdkit/core/ratelimit/facade"
	rlMiddleware "github.com/huwenlong92/sdkit/core/ratelimit/middleware"
	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestUseDefaultsToInternal(t *testing.T) {
	capability := ratelimitfacade.Use()
	if !capability.Metadata().Internal {
		t.Fatal("Use() should default to internal capability")
	}

	capability = ratelimitfacade.Use(ratelimitfacade.WithExternal())
	if capability.Metadata().Internal {
		t.Fatal("WithExternal() should expose capability")
	}
}

func TestUseWithoutRedisBindsMemoryStore(t *testing.T) {
	t.Cleanup(func() {
		rlMiddleware.SetStore(nil)
	})

	app := runtime.New()
	if err := app.Use(ratelimitfacade.Use()); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := ratelimitfacade.From(app); got == nil {
		t.Fatal("ratelimit.From(app) = nil, want store")
	}
	if rlMiddleware.CustomStore == nil {
		t.Fatal("middleware store = nil, want store")
	}
}

func TestUseWithStoreBindsInjectedStore(t *testing.T) {
	t.Cleanup(func() {
		rlMiddleware.SetStore(nil)
	})

	store := ratelimitfacade.NewMemoryStore()
	t.Cleanup(func() {
		_ = store.Close()
	})
	app := runtime.New()
	if err := app.Use(ratelimitfacade.Use(ratelimitfacade.WithStore(store))); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := ratelimitfacade.From(app); got != store {
		t.Fatalf("ratelimit.From(app) = %p, want %p", got, store)
	}
}
