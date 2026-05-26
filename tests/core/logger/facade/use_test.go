package facade_test

import (
	"context"
	"testing"

	corelogger "github.com/huwenlong92/sdkit/core/logger"
	loggerfacade "github.com/huwenlong92/sdkit/core/logger/facade"
	"github.com/huwenlong92/sdkit/core/runtime"

	"go.uber.org/zap"
)

func TestUseDefaultsToInternal(t *testing.T) {
	capability := loggerfacade.Use()
	if !capability.Metadata().Internal {
		t.Fatal("Use() should default to internal capability")
	}

	capability = loggerfacade.Use(loggerfacade.WithExternal())
	if capability.Metadata().Internal {
		t.Fatal("WithExternal() should expose capability")
	}
}

func TestUseWithLoggerBindsInjectedLogger(t *testing.T) {
	old := corelogger.L
	t.Cleanup(func() {
		corelogger.L = old
	})

	log := zap.NewNop()
	app := runtime.New()
	if err := app.Use(loggerfacade.Use(loggerfacade.WithLogger(log))); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := corelogger.From(app); got != log {
		t.Fatalf("logger.From(app) = %p, want %p", got, log)
	}
}
