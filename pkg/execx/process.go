package execx

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"time"
)

type Process struct {
	cmd    *exec.Cmd
	cfg    config
	cancel context.CancelCauseFunc
	ring   *RingSink

	done chan struct{}

	mu      sync.Mutex
	result  Result
	err     error
	running bool
}

func Start(ctx context.Context, name string, args []string, opts ...Option) (*Process, error) {
	cfg := applyOptions(opts)
	if cfg.ringBuffer == defaultRingBuffer {
		cfg.ringBuffer = defaultRingBuffer
	}
	if !cfg.killProcessGroup {
		cfg.killProcessGroup = true
	}
	cmdCtx, cancel := context.WithCancelCause(normalizeContext(ctx))
	cmd := newCommand(cmdCtx, name, args, cfg)
	result := newResult(name, args, cfg)

	var sink Sink
	ring := NewRingSink(cfg.ringBuffer)
	if cfg.sink != nil {
		sink = MultiSink{ring, cfg.sink}
	} else {
		sink = ring
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel(err)
		result.FinishedAt = time.Now()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel(err)
		result.FinishedAt = time.Now()
		return nil, err
	}

	if err := startCommand(cmd, &result, cfg); err != nil {
		cancel(err)
		result.FinishedAt = time.Now()
		return nil, err
	}

	p := &Process{
		cmd:     cmd,
		cfg:     cfg,
		cancel:  cancel,
		ring:    ring,
		done:    make(chan struct{}),
		result:  result,
		running: true,
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go p.read(cmdCtx, &wg, stdout, StreamStdout, sink)
	stderrStream := StreamStderr
	if cfg.mergeStderr {
		stderrStream = StreamStdout
	}
	go p.read(cmdCtx, &wg, stderr, stderrStream, sink)
	go p.wait(cmdCtx, &wg)
	return p, nil
}

func (p *Process) PID() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *Process) Done() <-chan Result {
	ch := make(chan Result, 1)
	if p == nil {
		close(ch)
		return ch
	}
	go func() {
		<-p.done
		p.mu.Lock()
		result := p.result
		p.mu.Unlock()
		ch <- result
		close(ch)
	}()
	return ch
}

func (p *Process) Wait() (Result, error) {
	if p == nil {
		return Result{}, nil
	}
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.result, p.err
}

func (p *Process) Stop(ctx context.Context) error {
	if p == nil || !p.Running() {
		return ErrProcessNotRunning
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), p.cfg.stopTimeout)
		defer cancel()
	}
	if err := terminateCommand(p.cmd, p.cfg.killProcessGroup); err != nil {
		return err
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		if err := p.Kill(); err != nil && !errors.Is(err, ErrProcessNotRunning) {
			return err
		}
		<-p.done
		return ctx.Err()
	}
}

func (p *Process) Kill() error {
	if p == nil || !p.Running() {
		return ErrProcessNotRunning
	}
	return killCommand(p.cmd, p.cfg.killProcessGroup)
}

func (p *Process) Signal(sig os.Signal) error {
	if p == nil || !p.Running() {
		return ErrProcessNotRunning
	}
	return signalCommand(p.cmd, sig, p.cfg.killProcessGroup)
}

func (p *Process) Running() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

func (p *Process) RecentEvents() []Event {
	if p == nil || p.ring == nil {
		return nil
	}
	return p.ring.Events()
}

func (p *Process) read(ctx context.Context, wg *sync.WaitGroup, r anyReader, stream Stream, sink Sink) {
	defer wg.Done()
	if err := readOutput(ctx, r, stream, sink, p.cfg); err != nil {
		p.cancel(err)
	}
}

func (p *Process) wait(ctx context.Context, wg *sync.WaitGroup) {
	waitErr := p.cmd.Wait()
	wg.Wait()
	p.mu.Lock()
	finishResult(p.cmd, &p.result)
	p.running = false
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) {
		p.err = cause
	} else {
		p.err = commandError(ctx, p.result, waitErr)
	}
	p.mu.Unlock()
	close(p.done)
}
