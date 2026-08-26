package baidupan

import (
	"context"

	"github.com/huwenlong92/sdkit/pkg/execx"
)

type Command struct {
	Name        string
	Args        []string
	Dir         string
	Env         []string
	Input       string
	Interactive bool
	OutputLimit int64
	SplitMode   execx.SplitMode
	MergeStderr bool
}

type Runner interface {
	RunOutput(ctx context.Context, command Command) (execx.OutputResult, error)
	RunStream(ctx context.Context, command Command, sink execx.Sink) (execx.Result, error)
}

type ExecRunner struct{}

func (ExecRunner) RunOutput(ctx context.Context, command Command) (execx.OutputResult, error) {
	return execx.RunOutput(ctx, command.Name, runnerArgs(command), commandOptions(command)...)
}

func (ExecRunner) RunStream(ctx context.Context, command Command, sink execx.Sink) (execx.Result, error) {
	return execx.RunStream(ctx, command.Name, runnerArgs(command), sink, commandOptions(command)...)
}

func runnerArgs(command Command) []string {
	if command.Interactive {
		return nil
	}
	return command.Args
}

func commandOptions(command Command) []execx.Option {
	options := []execx.Option{
		execx.WithCleanEnv(),
		execx.WithEnv(command.Env),
		execx.WithOutputLimit(command.OutputLimit),
		execx.WithSplitMode(command.SplitMode),
		execx.WithKillProcessGroup(),
	}
	if command.Dir != "" {
		options = append(options, execx.WithDir(command.Dir))
	}
	if command.Input != "" {
		options = append(options, execx.WithInputString(command.Input))
	}
	if command.MergeStderr {
		options = append(options, execx.WithMergeStderr())
	}
	return options
}
