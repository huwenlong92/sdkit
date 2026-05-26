package facade_test

import (
	"context"
	"testing"

	corecache "github.com/huwenlong92/sdkit/core/cache"
	cachefacade "github.com/huwenlong92/sdkit/core/cache/facade"
	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestUseDefaultsToInternal(t *testing.T) {
	capability := cachefacade.Use()
	if !capability.Metadata().Internal {
		t.Fatal("Use() should default to internal capability")
	}

	capability = cachefacade.Use(cachefacade.WithExternal())
	if capability.Metadata().Internal {
		t.Fatal("WithExternal() should expose capability")
	}
}

func TestUseWithoutConfigBindsDefaultCache(t *testing.T) {
	t.Cleanup(corecache.Close)

	app := runtime.New()
	if err := app.Use(cachefacade.Use()); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := corecache.From(app); got == nil {
		t.Fatal("cache.From(app) = nil, want cache instance")
	}
}

func TestUseWithCacheBindsInjectedCache(t *testing.T) {
	t.Cleanup(corecache.Close)

	cache := corecache.New()
	app := runtime.New()
	if err := app.Use(cachefacade.Use(cachefacade.WithCache(cache))); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := corecache.From(app); got != cache {
		t.Fatalf("cache.From(app) = %p, want %p", got, cache)
	}
}
