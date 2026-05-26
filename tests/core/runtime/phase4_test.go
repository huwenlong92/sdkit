package tests

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/huwenlong92/sdkit/core/runtime"
)

type phase4Command struct {
	name     string
	metadata runtime.CommandMetadata
	run      func(context.Context, *runtime.App, []string) error
}

func (c phase4Command) Name() string {
	return c.name
}

func (c phase4Command) Metadata() runtime.CommandMetadata {
	metadata := c.metadata
	if metadata.Name == "" {
		metadata.Name = c.name
	}
	return metadata
}

func (c phase4Command) Run(ctx context.Context, app *runtime.App, args []string) error {
	if c.run == nil {
		return nil
	}
	return c.run(ctx, app, args)
}

func TestPhase4RegisterCommandValidatesNames(t *testing.T) {
	t.Run("name required", func(t *testing.T) {
		app := runtime.New()
		if err := app.RegisterCommand(testCommand{}); !errors.Is(err, runtime.ErrCommandNameRequired) {
			t.Fatalf("RegisterCommand(empty name) error = %v, want ErrCommandNameRequired", err)
		}
	})

	for _, name := range []string{"command", "default", "main"} {
		t.Run("reserved "+name, func(t *testing.T) {
			app := runtime.New()
			if err := app.RegisterCommand(testCommand{name: name}); !errors.Is(err, runtime.ErrCommandNameReserved) {
				t.Fatalf("RegisterCommand(%q) error = %v, want ErrCommandNameReserved", name, err)
			}
		})
	}

	t.Run("duplicate existing", func(t *testing.T) {
		app := runtime.New()
		if err := app.RegisterCommand(testCommand{name: "serve"}); err != nil {
			t.Fatalf("RegisterCommand(serve) error = %v", err)
		}
		if err := app.RegisterCommand(testCommand{name: "serve"}); !errors.Is(err, runtime.ErrCommandNameDuplicate) {
			t.Fatalf("RegisterCommand(duplicate serve) error = %v, want ErrCommandNameDuplicate", err)
		}
		if got := len(app.Commands()); got != 1 {
			t.Fatalf("command count = %d, want 1", got)
		}
	})

	t.Run("duplicate in batch does not partially register", func(t *testing.T) {
		app := runtime.New()
		err := app.RegisterCommand(
			testCommand{name: "run"},
			testCommand{name: "run"},
		)
		if !errors.Is(err, runtime.ErrCommandNameDuplicate) {
			t.Fatalf("RegisterCommand(duplicate batch) error = %v, want ErrCommandNameDuplicate", err)
		}
		if got := len(app.Commands()); got != 0 {
			t.Fatalf("command count = %d, want 0", got)
		}
	})
}

func TestPhase4RunProviderStartsOnlyTargetProvider(t *testing.T) {
	app := runtime.New()
	var capabilityCount int
	if err := app.Use(runtime.NewCapability("shared", func(*runtime.App) error {
		capabilityCount++
		return nil
	})); err != nil {
		t.Fatalf("Use() error = %v", err)
	}

	var calls []string
	if err := app.Register(
		testProvider{
			name: "api",
			register: func(*runtime.App) error {
				calls = append(calls, "api.register")
				return nil
			},
			start: func(context.Context) error {
				calls = append(calls, "api.start")
				return nil
			},
		},
		testProvider{
			name: "worker",
			register: func(*runtime.App) error {
				calls = append(calls, "worker.register")
				return nil
			},
			start: func(context.Context) error {
				calls = append(calls, "worker.start")
				return nil
			},
		},
	); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.RunProvider("api", context.Background()); err != nil {
		t.Fatalf("RunProvider(api) error = %v", err)
	}
	if capabilityCount != 1 {
		t.Fatalf("capability count = %d, want 1", capabilityCount)
	}
	if want := []string{"api.register", "api.start"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestPhase4RunProviderRequiresExistingProvider(t *testing.T) {
	app := runtime.New()
	if err := app.RunProvider("api"); !errors.Is(err, runtime.ErrProviderNotFound) {
		t.Fatalf("RunProvider(missing) error = %v, want ErrProviderNotFound", err)
	}
}

func TestPhase4RunAllProvidersInitializesCapabilityOnce(t *testing.T) {
	app := runtime.New()
	var capabilityCount int
	if err := app.Use(runtime.NewCapability("shared", func(*runtime.App) error {
		capabilityCount++
		return nil
	})); err != nil {
		t.Fatalf("Use() error = %v", err)
	}

	var calls []string
	if err := app.Register(
		testProvider{
			name: "api",
			start: func(context.Context) error {
				calls = append(calls, "api.start")
				return nil
			},
		},
		testProvider{
			name: "worker",
			start: func(context.Context) error {
				calls = append(calls, "worker.start")
				return nil
			},
		},
	); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.RunAllProviders(context.Background()); err != nil {
		t.Fatalf("RunAllProviders() error = %v", err)
	}
	if capabilityCount != 1 {
		t.Fatalf("capability count = %d, want 1", capabilityCount)
	}
	if want := []string{"api.start", "worker.start"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestPhase4ExecuteDispatchesCommand(t *testing.T) {
	app := runtime.New()
	var gotArgs []string
	if err := app.RegisterCommand(phase4Command{
		name: "serve",
		run: func(ctx context.Context, app *runtime.App, args []string) error {
			gotArgs = append(gotArgs, args...)
			return nil
		},
	}); err != nil {
		t.Fatalf("RegisterCommand() error = %v", err)
	}

	if err := runtime.Execute(app, []string{"sdkitgo", "serve", "api"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if want := []string{"api"}; !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("command args = %v, want %v", gotArgs, want)
	}
}

func TestPhase4ExecuteReturnsCommandNotFound(t *testing.T) {
	app := runtime.New()
	if err := runtime.Execute(app, []string{"sdkitgo", "missing"}); !errors.Is(err, runtime.ErrCommandNotFound) {
		t.Fatalf("Execute(missing) error = %v, want ErrCommandNotFound", err)
	}
}
