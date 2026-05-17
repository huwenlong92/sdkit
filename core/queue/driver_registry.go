package queue

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

const defaultDriverName = "asynq"

type RunnerDriver interface {
	Driver
	NewRunner(cfg Config, opts ...RuntimeOption) (QueueRunner, error)
}

var (
	driverMu sync.RWMutex
	drivers  = map[string]RunnerDriver{}
)

func RegisterDriver(driver RunnerDriver) error {
	if driver == nil {
		return fmt.Errorf("queue: nil driver")
	}
	name := strings.TrimSpace(driver.Name())
	if name == "" {
		return fmt.Errorf("queue: driver name is required")
	}
	driverMu.Lock()
	drivers[name] = driver
	driverMu.Unlock()
	return nil
}

func NewRunner(cfg Config, opts ...RuntimeOption) (QueueRunner, error) {
	cfg = cfg.Normalize()
	driver, err := resolveDriver(cfg.Driver)
	if err != nil {
		return nil, err
	}
	runner, err := driver.NewRunner(cfg, opts...)
	if err != nil {
		return nil, err
	}
	if runner == nil {
		return nil, ErrNotInitialized
	}
	return runner, nil
}

func NewClient(cfg Config) (Client, error) {
	cfg = cfg.Normalize()
	driver, err := resolveDriver(cfg.Driver)
	if err != nil {
		return nil, err
	}
	client, err := driver.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, ErrNotInitialized
	}
	return client, nil
}

func NewManager(cfg Config) (Manager, error) {
	cfg = cfg.Normalize()
	driver, err := resolveDriver(cfg.Driver)
	if err != nil {
		return nil, err
	}
	manager, err := driver.NewManager(cfg)
	if err != nil {
		return nil, err
	}
	if manager == nil {
		return nil, ErrNotInitialized
	}
	return manager, nil
}

func resolveDriver(name string) (RunnerDriver, error) {
	name = normalizeDriverName(name)
	driverMu.RLock()
	driver := drivers[name]
	driverMu.RUnlock()
	if driver == nil {
		return nil, fmt.Errorf("queue: driver %q is not registered: %w", name, ErrCapabilityUnsupported)
	}
	return driver, nil
}

func normalizeDriverName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return defaultDriverName
	}
	return name
}

type unavailableRunner struct {
	err error
}

func newUnavailableRunner(err error) *unavailableRunner {
	if err == nil {
		err = ErrNotInitialized
	}
	return &unavailableRunner{err: err}
}

func (r *unavailableRunner) Err() error {
	if r == nil || r.err == nil {
		return ErrNotInitialized
	}
	return r.err
}

func (r *unavailableRunner) Enqueue(context.Context, Task, ...Option) (*TaskInfo, error) {
	return nil, r.Err()
}

func (r *unavailableRunner) BatchEnqueue(context.Context, []Task, ...Option) ([]*TaskInfo, error) {
	return nil, r.Err()
}

func (r *unavailableRunner) Close() error {
	return nil
}

func (r *unavailableRunner) Handle(string, HandlerFunc) {}

func (r *unavailableRunner) Use(...Middleware) {}

func (r *unavailableRunner) Run(context.Context) error {
	return r.Err()
}

func (r *unavailableRunner) Shutdown(context.Context) error {
	return nil
}

func (r *unavailableRunner) Supports(Capability) bool {
	return false
}

func (r *unavailableRunner) Capabilities() map[Capability]bool {
	return map[Capability]bool{}
}

func (r *unavailableRunner) ListQueues(context.Context) ([]*QueueInfo, error) {
	return nil, r.Err()
}

func (r *unavailableRunner) GetQueue(context.Context, string) (*QueueInfo, error) {
	return nil, r.Err()
}

func (r *unavailableRunner) ListTasks(context.Context, TaskQuery) ([]*TaskInfo, error) {
	return nil, r.Err()
}

func (r *unavailableRunner) GetTask(context.Context, string, string) (*TaskInfo, error) {
	return nil, r.Err()
}

func (r *unavailableRunner) DeleteTask(context.Context, string, string) error {
	return r.Err()
}

func (r *unavailableRunner) RetryTask(context.Context, string, string) error {
	return r.Err()
}

func (r *unavailableRunner) ArchiveTask(context.Context, string, string) error {
	return r.Err()
}

func (r *unavailableRunner) CancelTask(context.Context, string, string) error {
	return r.Err()
}

func (r *unavailableRunner) PauseQueue(context.Context, string) error {
	return r.Err()
}

func (r *unavailableRunner) ResumeQueue(context.Context, string) error {
	return r.Err()
}

var _ QueueRunner = (*unavailableRunner)(nil)
