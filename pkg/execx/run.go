package execx

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"time"
)

func Run(ctx context.Context, name string, args []string, opts ...Option) (Result, error) {
	cfg := applyOptions(opts)
	cmdCtx := normalizeContext(ctx)
	cmd := newCommand(cmdCtx, name, args, cfg)
	result := newResult(name, args, cfg)
	if err := startCommand(cmd, &result, cfg); err != nil {
		result.FinishedAt = time.Now()
		return result, err
	}
	err := cmd.Wait()
	finishResult(cmd, &result)
	return result, commandError(cmdCtx, result, err)
}

func RunOutput(ctx context.Context, name string, args []string, opts ...Option) (OutputResult, error) {
	cfg := applyOptions(opts)
	capture := newCaptureSink(cfg.outputLimit)
	runOpts := append([]Option(nil), opts...)
	runOpts = append(runOpts, WithSplitMode(SplitChunk))
	result, err := RunStream(ctx, name, args, capture, runOpts...)
	output := capture.Output()
	output.Result = result
	return output, err
}

func RunStream(ctx context.Context, name string, args []string, sink Sink, opts ...Option) (Result, error) {
	cfg := applyOptions(opts)
	if sink == nil {
		sink = SinkFunc(func(context.Context, Event) error { return nil })
	}
	cmdCtx, cancel := context.WithCancelCause(normalizeContext(ctx))
	defer cancel(nil)
	cmd := newCommand(cmdCtx, name, args, cfg)
	result := newResult(name, args, cfg)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		result.FinishedAt = time.Now()
		return result, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		result.FinishedAt = time.Now()
		return result, err
	}

	if err := startCommand(cmd, &result, cfg); err != nil {
		result.FinishedAt = time.Now()
		return result, err
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	read := func(r anyReader, stream Stream) {
		defer wg.Done()
		if err := readOutput(cmdCtx, r, stream, sink, cfg); err != nil {
			errCh <- err
			cancel(err)
		}
	}
	wg.Add(2)
	go read(stdout, StreamStdout)
	stderrStream := StreamStderr
	if cfg.mergeStderr {
		stderrStream = StreamStdout
	}
	go read(stderr, stderrStream)

	waitErr := cmd.Wait()
	wg.Wait()
	close(errCh)
	finishResult(cmd, &result)

	var readErr error
	for err := range errCh {
		readErr = errors.Join(readErr, err)
	}
	if cause := context.Cause(cmdCtx); cause != nil && !errors.Is(cause, context.Canceled) {
		return result, cause
	}
	if readErr != nil && cmdCtx.Err() == nil {
		return result, readErr
	}
	return result, commandError(cmdCtx, result, waitErr)
}

type anyReader interface {
	Read([]byte) (int, error)
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func newCommand(ctx context.Context, name string, args []string, cfg config) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cfg.dir
	cmd.Env = buildEnv(cfg)
	cmd.Stdin = cfg.stdin
	if cfg.waitDelay > 0 {
		cmd.WaitDelay = cfg.waitDelay
	}
	applyPlatformOptions(cmd, cfg)
	cmd.Cancel = func() error {
		return killCommand(cmd, cfg.killProcessGroup)
	}
	return cmd
}

func buildEnv(cfg config) []string {
	if cfg.cleanEnv {
		return append([]string(nil), cfg.env...)
	}
	if len(cfg.env) == 0 {
		return nil
	}
	env := os.Environ()
	env = append(env, cfg.env...)
	return env
}

func newResult(name string, args []string, cfg config) Result {
	return Result{
		Command:  name,
		Args:     append([]string(nil), args...),
		Dir:      cfg.dir,
		ExitCode: -1,
	}
}

func startCommand(cmd *exec.Cmd, result *Result, cfg config) error {
	result.StartedAt = time.Now()
	if err := cmd.Start(); err != nil {
		return err
	}
	if cmd.Process != nil {
		result.PID = cmd.Process.Pid
	}
	return nil
}

func finishResult(cmd *exec.Cmd, result *Result) {
	result.FinishedAt = time.Now()
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
}

func commandError(ctx context.Context, result Result, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &ExitError{Result: result, Err: err}
	}
	return err
}

type captureSink struct {
	mu       sync.Mutex
	limit    int64
	size     int64
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	combined bytes.Buffer
}

func newCaptureSink(limit int64) *captureSink {
	return &captureSink{limit: limit}
}

func (s *captureSink) WriteCommandEvent(ctx context.Context, event Event) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.size + int64(len(event.Data))
	if s.limit > 0 && next > s.limit {
		return ErrOutputLimitExceeded
	}
	s.size = next
	switch event.Stream {
	case StreamStderr:
		_, _ = s.stderr.Write(event.Data)
	default:
		_, _ = s.stdout.Write(event.Data)
	}
	_, _ = s.combined.Write(event.Data)
	return nil
}

func (s *captureSink) Output() OutputResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return OutputResult{
		Stdout:   append([]byte(nil), s.stdout.Bytes()...),
		Stderr:   append([]byte(nil), s.stderr.Bytes()...),
		Combined: append([]byte(nil), s.combined.Bytes()...),
	}
}
