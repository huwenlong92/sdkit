package tracing

import (
	"context"
	"reflect"
	"testing"

	"github.com/huwenlong92/sdkit/core/runtime"
	coretracing "github.com/huwenlong92/sdkit/core/tracing"
)

func TestUseRegistersTracingCapability(t *testing.T) {
	t.Cleanup(func() {
		_ = coretracing.Shutdown(context.Background())
	})

	capability := Use(WithConfig(Config{Enabled: false}))
	metadata := capability.Metadata()
	if metadata.Name != Name {
		t.Fatalf("metadata name = %q, want %q", metadata.Name, Name)
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
}

func TestUseConfigLoaderRunsAfterBootstrap(t *testing.T) {
	t.Cleanup(func() {
		_ = coretracing.Shutdown(context.Background())
	})

	app := runtime.New()
	order := make([]string, 0, 2)
	if err := app.RegisterCapabilities(
		Use(WithConfigLoader(func(app *runtime.App) (Config, error) {
			if _, ok := app.Container().Get(runtime.Key("config")); !ok {
				t.Fatal("config not initialized before tracing config loader")
			}
			order = append(order, "tracing.loader")
			return Config{Enabled: false}, nil
		})),
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

	want := []string{"bootstrap", "tracing.loader"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}
