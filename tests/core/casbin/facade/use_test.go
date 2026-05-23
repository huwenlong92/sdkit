package facade_test

import (
	"context"
	"reflect"
	"testing"

	corecasbin "github.com/huwenlong92/sdkit/core/casbin"
	casbinfacade "github.com/huwenlong92/sdkit/core/casbin/facade"
	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestUseRegistersCasbinCapability(t *testing.T) {
	t.Cleanup(func() { corecasbin.Default = nil })

	manager := &corecasbin.Manager{}
	capability := casbinfacade.Use(casbinfacade.WithCapabilityManager(manager))

	metadata := capability.Metadata()
	if metadata.Name != string(casbinfacade.KeyCasbin) {
		t.Fatalf("metadata name = %q, want %q", metadata.Name, casbinfacade.KeyCasbin)
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
	if got := casbinfacade.From(app); got != manager {
		t.Fatalf("From(app) = %p, want %p", got, manager)
	}
	if got := casbinfacade.Default(); got != manager {
		t.Fatalf("Default() = %p, want %p", got, manager)
	}
}

func TestUseConfigLoaderRunsAfterBootstrap(t *testing.T) {
	t.Cleanup(func() { corecasbin.Default = nil })

	app := runtime.New()
	manager := &corecasbin.Manager{}
	order := make([]string, 0, 2)
	if err := app.RegisterCapabilities(
		casbinfacade.Use(
			casbinfacade.WithCapabilityManager(manager),
			casbinfacade.WithConfigLoader(func(app *runtime.App) (casbinfacade.Config, error) {
				if _, ok := app.Container().Get(runtime.Key("config")); !ok {
					t.Fatal("config not initialized before casbin config loader")
				}
				order = append(order, "casbin.loader")
				return casbinfacade.Config{ModelPath: "configs/rbac_model.conf"}, nil
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

	want := []string{"bootstrap", "casbin.loader"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}
