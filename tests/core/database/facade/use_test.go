package facade_test

import (
	"context"
	"errors"
	"testing"

	coredatabase "github.com/huwenlong92/sdkit/core/database"
	databasefacade "github.com/huwenlong92/sdkit/core/database/facade"
	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestUseDefaultsToInternal(t *testing.T) {
	capability := databasefacade.Use()
	if !capability.Metadata().Internal {
		t.Fatal("Use() should default to internal capability")
	}

	capability = databasefacade.Use(databasefacade.WithExternal())
	if capability.Metadata().Internal {
		t.Fatal("WithExternal() should expose capability")
	}
}

func TestUseRequiresConfigOrDatabase(t *testing.T) {
	app := runtime.New()
	if err := app.Use(databasefacade.Use()); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); !errors.Is(err, databasefacade.ErrConfigRequired) {
		t.Fatalf("Run() error = %v, want ErrConfigRequired", err)
	}
}

func TestUseWithDatabaseSkipsConfig(t *testing.T) {
	t.Cleanup(coredatabase.Close)

	db := &coredatabase.Database{}
	app := runtime.New()
	if err := app.Use(databasefacade.Use(databasefacade.WithDatabase(db))); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := coredatabase.From(app); got != db {
		t.Fatalf("database.From(app) = %p, want %p", got, db)
	}
}
