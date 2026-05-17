package database

import (
	"context"
	"reflect"
	"testing"

	coredatabase "github.com/huwenlong92/sdkit/core/database"
	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestUseRegistersDatabaseCapability(t *testing.T) {
	t.Cleanup(coredatabase.Close)

	db := &coredatabase.Database{}
	capability := Use(WithDatabase(db))

	metadata := capability.Metadata()
	if metadata.Name != string(KeyDatabase) {
		t.Fatalf("metadata name = %q, want %q", metadata.Name, KeyDatabase)
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
	if got := From(app); got != db {
		t.Fatalf("From(app) = %p, want %p", got, db)
	}
}

func TestUseConfigLoaderRunsAfterBootstrap(t *testing.T) {
	t.Cleanup(coredatabase.Close)

	app := runtime.New()
	db := &coredatabase.Database{}
	order := make([]string, 0, 2)
	if err := app.RegisterCapabilities(
		Use(
			WithDatabase(db),
			WithConfigLoader(func(app *runtime.App) (Config, error) {
				if _, ok := app.Container().Get(runtime.Key("config")); !ok {
					t.Fatal("config not initialized before database config loader")
				}
				order = append(order, "database.loader")
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

	want := []string{"bootstrap", "database.loader"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}
