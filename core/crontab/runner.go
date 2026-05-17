package crontab

import (
	"context"
	"os"
	"time"
)

type Runner struct {
	registry *Registry
	store    Store
	locker   Locker
	logger   LogWriter
	logStore LogStore
	streamer LogStreamer
	runtime  *RuntimeState
	cfg      Config
}

type RunnerOptions struct {
	Config   Config
	Registry *Registry
	Store    Store
	Locker   Locker
	Logger   LogWriter
	LogStore LogStore
	Streamer LogStreamer
	Runtime  *RuntimeState
}

func NewRunner(opts RunnerOptions) *Runner {
	if opts.Store == nil {
		opts.Store = NoopStore{}
	}
	if opts.Locker == nil {
		opts.Locker = NoopLocker{}
	}
	if opts.Logger == nil {
		opts.Logger = NoopLogWriter{}
	}
	if !opts.Config.Log.Enabled {
		opts.Logger = NoopLogWriter{}
	}
	if opts.Config.InstanceID == "" {
		if hostname, err := os.Hostname(); err == nil {
			opts.Config.InstanceID = hostname
		}
	}

	return &Runner{
		registry: opts.Registry,
		store:    opts.Store,
		locker:   opts.Locker,
		logger:   opts.Logger,
		logStore: opts.LogStore,
		streamer: opts.Streamer,
		runtime:  opts.Runtime,
		cfg:      opts.Config,
	}
}

func (r *Runner) Run(ctx context.Context, job Job) {
	if r == nil {
		return
	}
	_ = r.execute(newRunContext(ctx, r, job))
}

func (r *Runner) execute(c *RunContext) RunResult {
	if c == nil {
		return RunResult{Status: StatusFailed, Err: ErrTemplateNotFound}
	}
	return r.executeTemplate(c)
}

func (r *Runner) executeTemplate(c *RunContext) RunResult {
	if c.runner == nil || c.runner.registry == nil {
		return RunResult{Status: StatusFailed, Err: ErrRegistryRequired}
	}
	err := c.runner.registry.Dispatch(ContextWithRunContext(c.Context(), c), &c.Entry)
	if err != nil {
		return runResultFromError(err)
	}
	return RunResult{Status: StatusSuccess}
}

func (r *Runner) refreshLock(ctx context.Context, lock Lock, ttl time.Duration) func() {
	if lock == nil || ttl <= 0 {
		return func() {}
	}
	interval := ttl / 3
	if interval <= 0 {
		return func() {}
	}
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = lock.Refresh(ctx, ttl)
			case <-stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() { close(stop) }
}

func (r *Runner) LastRunStatus(jobID string) Status {
	if r == nil || r.runtime == nil {
		return ""
	}
	info, ok := r.runtime.Get(jobID)
	if !ok {
		return ""
	}
	switch info.Status {
	case RuntimeSuccess:
		return StatusSuccess
	case RuntimeRunning:
		return StatusRunning
	case RuntimeSkipped:
		if info.LastError == ErrJobRunning.Error() {
			return StatusLocked
		}
		return StatusSkipped
	case RuntimeDisabled:
		return StatusDisabled
	case RuntimeFailed:
		return StatusFailed
	default:
		return ""
	}
}

func runtimeStatus(status Status) RuntimeStatus {
	switch status {
	case StatusSuccess:
		return RuntimeSuccess
	case StatusRunning:
		return RuntimeRunning
	case StatusSkipped, StatusLocked, StatusTemplateDisabled, StatusTemplateMissing:
		return RuntimeSkipped
	case StatusDisabled:
		return RuntimeDisabled
	default:
		return RuntimeFailed
	}
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}
