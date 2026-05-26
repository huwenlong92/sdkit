package asynq

import (
	"fmt"
	"sync"

	"github.com/huwenlong92/sdkit/core/queue"
)

type Driver struct{}

var (
	registerOnce sync.Once
	registerErr  error
)

func NewDriver() Driver {
	return Driver{}
}

func Register() error {
	registerOnce.Do(func() {
		registerErr = queue.RegisterDriver(NewDriver())
	})
	return registerErr
}

func (Driver) Name() string {
	return "asynq"
}

func (d Driver) Capabilities() map[queue.Capability]bool {
	return capabilities()
}

func (d Driver) Supports(cap queue.Capability) bool {
	return d.Capabilities()[cap]
}

func (d Driver) NewClient(cfg queue.Config) (queue.Client, error) {
	return New(cfg), nil
}

func (d Driver) NewWorker(cfg queue.Config, profile queue.WorkerProfile) (queue.Worker, error) {
	cfg = cfg.Normalize()
	profile = profile.Normalize(profile.Name, cfg)
	cfg.Concurrency = profile.Concurrency
	cfg.Queues = profile.Queues
	cfg.StrictPriority = profile.StrictPriority
	return New(cfg), nil
}

func (d Driver) NewManager(cfg queue.Config) (queue.Manager, error) {
	return New(cfg), nil
}

func (d Driver) NewRunner(cfg queue.Config, opts ...queue.RuntimeOption) (queue.QueueRunner, error) {
	return New(cfg, opts...), nil
}

func unsupported(driver string, cap queue.Capability) error {
	return fmt.Errorf("queue driver %s does not support capability: %s: %w", driver, cap, queue.ErrCapabilityUnsupported)
}

func capabilities() map[queue.Capability]bool {
	return map[queue.Capability]bool{
		queue.CapEnqueue:     true,
		queue.CapConsume:     true,
		queue.CapRetry:       true,
		queue.CapTimeout:     true,
		queue.CapDeadline:    true,
		queue.CapDelay:       true,
		queue.CapUnique:      true,
		queue.CapPauseResume: true,
		queue.CapDLQ:         true,
		queue.CapInspector:   true,
		queue.CapBatch:       true,
		queue.CapLog:         true,
		queue.CapTrace:       true,
	}
}
