package facade_test

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/runtime"
	coresms "github.com/huwenlong92/sdkit/core/sms"
	smsfacade "github.com/huwenlong92/sdkit/core/sms/facade"
)

func TestUseDefaultsToInternal(t *testing.T) {
	capability := smsfacade.Use()
	if !capability.Metadata().Internal {
		t.Fatal("Use() should default to internal capability")
	}

	capability = smsfacade.Use(smsfacade.WithExternal())
	if capability.Metadata().Internal {
		t.Fatal("WithExternal() should expose capability")
	}
}

func TestUseWithoutConfigRequiresExplicitConfig(t *testing.T) {
	_ = coresms.Close()
	t.Cleanup(func() { _ = coresms.Close() })

	app := runtime.New()
	if err := app.Use(smsfacade.Use()); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	err := app.Run(context.Background())
	if !errors.Is(err, coresms.ErrNotConfigured) {
		t.Fatalf("Run() error = %v, want %v", err, coresms.ErrNotConfigured)
	}
}

func TestUseOptionalWithoutConfigSkipsBind(t *testing.T) {
	_ = coresms.Close()
	t.Cleanup(func() { _ = coresms.Close() })

	app := runtime.New()
	if err := app.Use(smsfacade.Use(smsfacade.WithOptional())); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := smsfacade.From(app); got != nil {
		t.Fatalf("From(app) = %p, want nil", got)
	}
}

func TestUseWithManagerBindsInjectedManager(t *testing.T) {
	_ = coresms.Close()
	t.Cleanup(func() { _ = coresms.Close() })

	manager := &coresms.Manager{}
	app := runtime.New()
	if err := app.Use(smsfacade.Use(smsfacade.WithManager(manager))); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := smsfacade.From(app); got != manager {
		t.Fatalf("From(app) = %p, want %p", got, manager)
	}
}
