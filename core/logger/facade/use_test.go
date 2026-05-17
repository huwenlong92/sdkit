package logger

import (
	"context"
	"reflect"
	"testing"

	corelogger "github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/runtime"

	"go.uber.org/zap"
)

func TestUseRegistersLoggerCapability(t *testing.T) {
	restoreLogger(t)

	app := runtime.New()
	log := zap.NewNop()
	capability := Use(WithLogger(log))

	metadata := capability.Metadata()
	if metadata.Name != string(KeyLogger) {
		t.Fatalf("metadata name = %q, want %q", metadata.Name, KeyLogger)
	}
	if metadata.Group != runtime.GroupSystem {
		t.Fatalf("metadata group = %q, want %q", metadata.Group, runtime.GroupSystem)
	}
	if metadata.Scope != runtime.ScopeGlobal {
		t.Fatalf("metadata scope = %q, want %q", metadata.Scope, runtime.ScopeGlobal)
	}

	if err := app.RegisterCapabilities(capability); err != nil {
		t.Fatalf("RegisterCapabilities() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := From(app); got != log {
		t.Fatalf("From(app) = %v, want injected logger", got)
	}
}

func TestUseConfigLoaderRunsAfterBootstrap(t *testing.T) {
	restoreLogger(t)

	app := runtime.New()
	order := make([]string, 0, 2)
	if err := app.RegisterCapabilities(
		Use(WithConfigLoader(func(app *runtime.App) (Config, error) {
			if _, ok := app.Container().Get(runtime.Key("config")); !ok {
				t.Fatal("config not initialized before logger config loader")
			}
			order = append(order, "logger.loader")
			return Config{
				Name:    "logger-loader-test",
				Level:   "error",
				Mode:    "test",
				Format:  "console",
				RootDir: t.TempDir(),
			}, nil
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

	want := []string{"bootstrap", "logger.loader"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func restoreLogger(t *testing.T) {
	t.Helper()
	prevLogger := corelogger.L
	corelogger.L = zap.NewNop()
	t.Cleanup(func() {
		corelogger.Sync()
		corelogger.L = prevLogger
	})
}
