package execx

import "context"

func RunShell(ctx context.Context, script string, opts ...Option) (Result, error) {
	shellOpts := withShellProcessGroup(opts)
	name, args := shellCommand(script, applyOptions(shellOpts))
	return Run(ctx, name, args, shellOpts...)
}

func RunShellOutput(ctx context.Context, script string, opts ...Option) (OutputResult, error) {
	shellOpts := withShellProcessGroup(opts)
	name, args := shellCommand(script, applyOptions(shellOpts))
	return RunOutput(ctx, name, args, shellOpts...)
}

func RunShellStream(ctx context.Context, script string, sink Sink, opts ...Option) (Result, error) {
	shellOpts := withShellProcessGroup(opts)
	name, args := shellCommand(script, applyOptions(shellOpts))
	return RunStream(ctx, name, args, sink, shellOpts...)
}

func StartShell(ctx context.Context, script string, opts ...Option) (*Process, error) {
	shellOpts := withShellProcessGroup(opts)
	name, args := shellCommand(script, applyOptions(shellOpts))
	return Start(ctx, name, args, shellOpts...)
}

func withShellProcessGroup(opts []Option) []Option {
	shellOpts := make([]Option, 0, len(opts)+1)
	shellOpts = append(shellOpts, WithKillProcessGroup())
	shellOpts = append(shellOpts, opts...)
	return shellOpts
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
