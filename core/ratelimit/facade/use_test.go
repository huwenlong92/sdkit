package ratelimit

import (
	"context"
	"reflect"
	"testing"

	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestUseRegistersRateLimitCapability(t *testing.T) {
	store := NewMemoryStore()
	t.Cleanup(func() {
		_ = store.Close()
		SetStore(nil)
	})
	capability := Use(WithStore(store))

	metadata := capability.Metadata()
	if metadata.Name != string(KeyRateLimit) {
		t.Fatalf("metadata name = %q, want %q", metadata.Name, KeyRateLimit)
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
	if got := From(app); got != store {
		t.Fatalf("From(app) = %v, want injected store", got)
	}
}

func TestUseConfigLoaderRunsAfterBootstrap(t *testing.T) {
	store := NewMemoryStore()
	t.Cleanup(func() {
		_ = store.Close()
		SetStore(nil)
	})

	app := runtime.New()
	order := make([]string, 0, 2)
	if err := app.RegisterCapabilities(
		Use(
			WithStore(store),
			WithConfigLoader(func(app *runtime.App) (Config, error) {
				if _, ok := app.Container().Get(runtime.Key("config")); !ok {
					t.Fatal("config not initialized before ratelimit config loader")
				}
				order = append(order, "ratelimit.loader")
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

	want := []string{"bootstrap", "ratelimit.loader"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}
