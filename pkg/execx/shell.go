package execx

import "context"

func RunShell(ctx context.Context, script string, opts ...Option) (Result, error) {
	name, args := shellCommand(script, applyOptions(opts))
	return Run(ctx, name, args, opts...)
}

func RunShellOutput(ctx context.Context, script string, opts ...Option) (OutputResult, error) {
	name, args := shellCommand(script, applyOptions(opts))
	return RunOutput(ctx, name, args, opts...)
}

func RunShellStream(ctx context.Context, script string, sink Sink, opts ...Option) (Result, error) {
	name, args := shellCommand(script, applyOptions(opts))
	return RunStream(ctx, name, args, sink, opts...)
}

func StartShell(ctx context.Context, script string, opts ...Option) (*Process, error) {
	name, args := shellCommand(script, applyOptions(opts))
	return Start(ctx, name, args, opts...)
}

func shellCommand(script string, cfg config) (string, []string) {
	name, args := defaultShell()
	if cfg.shellName != "" {
		name = cfg.shellName
		args = append([]string(nil), cfg.shellArgs...)
	}
	args = append(args, script)
	return name, args
}
