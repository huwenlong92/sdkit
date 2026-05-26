package facade_test

import (
	"context"
	"errors"
	"testing"

	coreemail "github.com/huwenlong92/sdkit/core/email"
	emailfacade "github.com/huwenlong92/sdkit/core/email/facade"
	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestUseDefaultsToInternal(t *testing.T) {
	capability := emailfacade.Use()
	if !capability.Metadata().Internal {
		t.Fatal("Use() should default to internal capability")
	}

	capability = emailfacade.Use(emailfacade.WithExternal())
	if capability.Metadata().Internal {
		t.Fatal("WithExternal() should expose capability")
	}
}

func TestUseWithoutConfigRequiresExplicitConfig(t *testing.T) {
	_ = coreemail.Close()
	t.Cleanup(func() { _ = coreemail.Close() })

	app := runtime.New()
	if err := app.Use(emailfacade.Use()); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	err := app.Run(context.Background())
	if !errors.Is(err, coreemail.ErrNotConfigured) {
		t.Fatalf("Run() error = %v, want %v", err, coreemail.ErrNotConfigured)
	}
}

func TestUseOptionalWithoutConfigSkipsBind(t *testing.T) {
	_ = coreemail.Close()
	t.Cleanup(func() { _ = coreemail.Close() })

	app := runtime.New()
	if err := app.Use(emailfacade.Use(emailfacade.WithOptional())); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := emailfacade.From(app); got != nil {
		t.Fatalf("From(app) = %p, want nil", got)
	}
}

func TestUseWithManagerBindsInjectedManager(t *testing.T) {
	_ = coreemail.Close()
	t.Cleanup(func() { _ = coreemail.Close() })

	manager := &coreemail.Manager{}
	app := runtime.New()
	if err := app.Use(emailfacade.Use(emailfacade.WithManager(manager))); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := emailfacade.From(app); got != manager {
		t.Fatalf("From(app) = %p, want %p", got, manager)
	}
}
