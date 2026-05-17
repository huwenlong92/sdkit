package redis

import (
	"context"
	"reflect"
	"testing"

	coreredis "github.com/huwenlong92/sdkit/core/redis"
	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestUseRegistersRedisCapability(t *testing.T) {
	t.Cleanup(func() {
		_ = coreredis.Close()
	})

	client := &coreredis.RuntimeClient{}
	capability := Use(WithClient(client))

	metadata := capability.Metadata()
	if metadata.Name != string(KeyRedis) {
		t.Fatalf("metadata name = %q, want %q", metadata.Name, KeyRedis)
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
	if got := From(app); got != client {
		t.Fatalf("From(app) = %p, want %p", got, client)
	}
}

func TestUseConfigLoaderRunsAfterBootstrap(t *testing.T) {
	t.Cleanup(func() {
		_ = coreredis.Close()
	})

	app := runtime.New()
	client := &coreredis.RuntimeClient{}
	order := make([]string, 0, 2)
	if err := app.RegisterCapabilities(
		Use(
			WithClient(client),
			WithConfigLoader(func(app *runtime.App) (Config, error) {
				if _, ok := app.Container().Get(runtime.Key("config")); !ok {
					t.Fatal("config not initialized before redis config loader")
				}
				order = append(order, "redis.loader")
				return Config{Addr: "127.0.0.1:6379"}, nil
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

	want := []string{"bootstrap", "redis.loader"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}
