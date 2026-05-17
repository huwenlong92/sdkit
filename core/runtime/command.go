package runtime

import (
	"context"
	"errors"
)

var (
	ErrCommandNil           = errors.New("runtime: command is nil")
	ErrCommandNameRequired  = errors.New("runtime: command name is required")
	ErrCommandNameReserved  = errors.New("runtime: command name is reserved")
	ErrCommandNameDuplicate = errors.New("runtime: command name is duplicate")
	ErrCommandNotFound      = errors.New("runtime: command not found")
)

type Command interface {
	Name() string
	Metadata() CommandMetadata
	Run(ctx context.Context, app *App, args []string) error
}

func isReservedCommandName(name string) bool {
	switch name {
	case "command", "default", "main":
		return true
	default:
		return false
	}
}
