package cache

import (
	"context"
	"reflect"
	"testing"

	corecache "github.com/huwenlong92/sdkit/core/cache"
	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestUseRegistersCacheCapability(t *testing.T) {
	t.Cleanup(corecache.Close)

	c := corecache.New()
	capability := Use(WithCache(c))

	metadata := capability.Metadata()
	if metadata.Name != string(KeyCache) {
		t.Fatalf("metadata name = %q, want %q", metadata.Name, KeyCache)
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
	if got := corecache.From(app); got != c {
		t.Fatalf("corecache.From(app) = %v, want injected cache", got)
	}
}

func TestUseConfigLoaderRunsAfterBootstrap(t *testing.T) {
	t.Cleanup(corecache.Close)

	app := runtime.New()
	c := corecache.New()
	order := make([]string, 0, 2)
	if err := app.RegisterCapabilities(
		Use(
			WithCache(c),
			WithConfigLoader(func(app *runtime.App) (Config, error) {
				if _, ok := app.Container().Get(runtime.Key("config")); !ok {
					t.Fatal("config not initialized before cache config loader")
				}
				order = append(order, "cache.loader")
				return Config{Prefix: "runtime-test:"}, nil
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

	want := []string{"bootstrap", "cache.loader"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}
