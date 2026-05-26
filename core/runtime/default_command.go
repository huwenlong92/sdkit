package runtime

import (
	"context"
	"errors"
	"fmt"
)

type serveCommand struct{}

func NewServeCommand() Command {
	return serveCommand{}
}

func (serveCommand) Name() string {
	return "serve"
}

func (serveCommand) Metadata() CommandMetadata {
	return CommandMetadata{
		Name:        "serve",
		Description: "Run all runtime providers",
		Group:       GroupSystem,
	}
}

func (serveCommand) Run(ctx context.Context, app *App, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("serve: unexpected args: %v", args)
	}
	if app == nil {
		return ErrAppNil
	}
	if !hasRuntimeManagedProvider(app.Providers()) {
		return RunAllProviders(ctx, app)
	}
	return runAllProvidersAndWait(ctx, app)
}

type runCommand struct{}

func NewRunCommand() Command {
	return runCommand{}
}

func (runCommand) Name() string {
	return "run"
}

func (runCommand) Metadata() CommandMetadata {
	return CommandMetadata{
		Name:        "run",
		Description: "Run one runtime provider",
		Group:       GroupSystem,
	}
}

func (runCommand) Run(ctx context.Context, app *App, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("run: provider name is required")
	}
	if app == nil {
		return ErrAppNil
	}
	provider, ok := app.Provider(args[0])
	if !ok || !isRuntimeManagedProvider(provider) {
		return RunProvider(ctx, app, args[0])
	}
	return runProviderAndWait(ctx, app, args[0])
}

type runtimeManagedProvider interface {
	RuntimeManaged() bool
}

func runAllProvidersAndWait(ctx context.Context, app *App) error {
	if app == nil {
		return ErrAppNil
	}
	if err := app.RunAllProviders(ctx); err != nil {
		return errors.Join(err, app.Stop(context.Background()))
	}
	return waitRuntimeStop(ctx, app)
}

func runProviderAndWait(ctx context.Context, app *App, name string) error {
	if app == nil {
		return ErrAppNil
	}
	if err := app.RunProvider(name, ctx); err != nil {
		return errors.Join(err, app.Stop(context.Background()))
	}
	return waitRuntimeStop(ctx, app)
}

func waitRuntimeStop(ctx context.Context, app *App) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return errors.Join(ctx.Err(), app.Stop(context.Background()))
	case <-app.Context().Done():
		return nil
	}
}

func hasRuntimeManagedProvider(providers []Provider) bool {
	for _, provider := range providers {
		if isRuntimeManagedProvider(provider) {
			return true
		}
	}
	return false
}

func isRuntimeManagedProvider(provider Provider) bool {
	managed, ok := provider.(runtimeManagedProvider)
	return ok && managed.RuntimeManaged()
}
