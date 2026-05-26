package facade_test

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/runtime"
	coretracing "github.com/huwenlong92/sdkit/core/tracing"
	tracingfacade "github.com/huwenlong92/sdkit/core/tracing/facade"
)

func TestUseDefaultsToInternal(t *testing.T) {
	capability := tracingfacade.Use()
	if !capability.Metadata().Internal {
		t.Fatal("Use() should default to internal capability")
	}

	capability = tracingfacade.Use(tracingfacade.WithExternal())
	if capability.Metadata().Internal {
		t.Fatal("WithExternal() should expose capability")
	}
}

func TestUseWithoutConfigInitializesNoopTracing(t *testing.T) {
	t.Cleanup(func() {
		_ = coretracing.Shutdown(context.Background())
	})

	app := runtime.New()
	if err := app.Use(tracingfacade.Use()); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if coretracing.Enabled() {
		t.Fatal("tracing should be disabled by default")
	}
}
