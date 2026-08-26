package ffprobe

import (
	"context"

	"github.com/huwenlong92/sdkit/pkg/execx"
)

type Command struct {
	Name        string
	Args        []string
	OutputLimit int64
}

type Result struct {
	Stdout []byte
	Stderr []byte
}

type Runner interface {
	RunOutput(ctx context.Context, command Command) (Result, error)
}

type ExecRunner struct{}

func (ExecRunner) RunOutput(ctx context.Context, command Command) (Result, error) {
	result, err := execx.RunOutput(
		ctx,
		command.Name,
		command.Args,
		execx.WithOutputLimit(command.OutputLimit),
		execx.WithKillProcessGroup(),
	)
	return Result{Stdout: result.Stdout, Stderr: result.Stderr}, err
}
