package runtime

import (
	"context"
	"errors"
	"fmt"
)

func Execute(app *App, args []string) error {
	if app == nil {
		return ErrAppNil
	}
	return ExecuteContext(app.Context(), app, args)
}

func ExecuteContext(ctx context.Context, app *App, args []string) error {
	if app == nil {
		return ErrAppNil
	}
	commandArgs := normalizeCommandArgs(app, args)
	if len(commandArgs) == 0 {
		return ErrCommandNameRequired
	}
	command, ok := app.Command(commandArgs[0])
	if !ok {
		return fmt.Errorf("%w: %s", ErrCommandNotFound, commandArgs[0])
	}
	if ctx == nil {
		ctx = app.Context()
	}
	return command.Run(ctx, app, commandArgs[1:])
}

func RunProvider(ctx context.Context, app *App, name string) error {
	if app == nil {
		return ErrAppNil
	}
	err := app.RunProvider(name, ctx)
	return errors.Join(err, app.Stop(context.Background()))
}

func RunAllProviders(ctx context.Context, app *App) error {
	if app == nil {
		return ErrAppNil
	}
	err := app.RunAllProviders(ctx)
	return errors.Join(err, app.Stop(context.Background()))
}

func normalizeCommandArgs(app *App, args []string) []string {
	if len(args) == 0 {
		return nil
	}
	if _, ok := app.Command(args[0]); ok {
		return args
	}
	if len(args) > 1 {
		if _, ok := app.Command(args[1]); ok {
			return args[1:]
		}
	}
	return args
}
