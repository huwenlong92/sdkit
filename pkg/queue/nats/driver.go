package nats

import (
	"fmt"
	"sync"

	corequeue "github.com/huwenlong92/sdkit/core/queue"
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
		registerErr = corequeue.RegisterDriver(NewDriver())
	})
	return registerErr
}

func (Driver) Name() string {
	return "nats"
}

func (d Driver) Capabilities() map[corequeue.Capability]bool {
	return capabilities()
}

func (d Driver) Supports(cap corequeue.Capability) bool {
	return d.Capabilities()[cap]
}

func (d Driver) NewClient(cfg corequeue.Config) (corequeue.Client, error) {
	return New(cfg)
}

func (d Driver) NewWorker(cfg corequeue.Config, profile corequeue.WorkerProfile) (corequeue.Worker, error) {
	cfg = cfg.Normalize()
	profile = profile.Normalize(profile.Name, cfg)
	cfg.Concurrency = profile.Concurrency
	cfg.Queues = profile.Queues
	return New(cfg)
}

func (d Driver) NewManager(cfg corequeue.Config) (corequeue.Manager, error) {
	return New(cfg)
}

func (d Driver) NewRunner(cfg corequeue.Config, opts ...corequeue.RuntimeOption) (corequeue.QueueRunner, error) {
	return New(cfg)
}

func unsupported(cap corequeue.Capability) error {
	return fmt.Errorf("queue driver nats does not support capability: %s: %w", cap, corequeue.ErrCapabilityUnsupported)
}

func capabilities() map[corequeue.Capability]bool {
	return map[corequeue.Capability]bool{
		corequeue.CapEnqueue: true,
		corequeue.CapConsume: true,
		corequeue.CapBatch:   true,
		corequeue.CapLog:     true,
		corequeue.CapTrace:   true,
	}
}
