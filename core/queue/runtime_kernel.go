package queue

import (
	"context"
	"fmt"
)

type OutboxFactory func(QueueRunner) (Outbox, error)

type RuntimeKernelConfig struct {
	IsFailure IsFailureFunc

	RateLimiter     RateLimiter
	RateLimitConfig RateLimitConfig

	OutboxFactory OutboxFactory
	OutboxMigrate func(context.Context) error
	OutboxPoller  OutboxPollerConfig

	EventPublishers []EventPublisher
	Observers       []Observer
}

type RuntimeKernel struct {
	runner QueueRunner

	cancelOutboxPoller context.CancelFunc

	rateLimiter RateLimiter
	rateLimit   RateLimitConfig

	orchestrator *Orchestrator
}

func InitRuntimeKernel(ctx context.Context, cfg Config, runtimeCfg RuntimeKernelConfig) (*RuntimeKernel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	kernel := &RuntimeKernel{}
	orchestratorOpts := make([]OrchestratorOption, 0, len(runtimeCfg.EventPublishers)+len(runtimeCfg.Observers))
	for _, publisher := range runtimeCfg.EventPublishers {
		if publisher != nil {
			orchestratorOpts = append(orchestratorOpts, WithEventPublisher(publisher))
		}
	}
	for _, observer := range runtimeCfg.Observers {
		if observer != nil {
			orchestratorOpts = append(orchestratorOpts, WithObserver(observer))
		}
	}
	kernel.orchestrator = NewOrchestrator(orchestratorOpts...)

	opts := make([]RuntimeOption, 0, 1)
	if runtimeCfg.IsFailure != nil {
		opts = append(opts, WithIsFailure(runtimeCfg.IsFailure))
	}

	runner, err := NewRunner(cfg, opts...)
	if err != nil {
		kernel.Close()
		return nil, err
	}
	kernel.runner = runner
	kernel.rateLimiter = runtimeCfg.RateLimiter
	kernel.rateLimit = normalizeRateLimitConfig(runtimeCfg.RateLimitConfig)

	if cfg.Outbox.Enabled && runtimeCfg.OutboxFactory != nil {
		if runtimeCfg.OutboxMigrate != nil {
			if err := runtimeCfg.OutboxMigrate(ctx); err != nil {
				kernel.Close()
				_ = runner.Close()
				return nil, fmt.Errorf("queue outbox migrate: %w", err)
			}
		}
		outbox, err := runtimeCfg.OutboxFactory(runner)
		if err != nil {
			kernel.Close()
			_ = runner.Close()
			return nil, fmt.Errorf("queue outbox factory: %w", err)
		}
		poller := NewOutboxPoller(outbox, runtimeCfg.OutboxPoller)
		kernel.cancelOutboxPoller = poller.Start(ctx)
	}

	return kernel, nil
}

func (k *RuntimeKernel) Runner() QueueRunner {
	if k == nil {
		return nil
	}
	return k.runner
}

func (k *RuntimeKernel) RateLimiter() (RateLimiter, RateLimitConfig, bool) {
	if k == nil || k.rateLimiter == nil || !k.rateLimit.Enabled {
		return nil, RateLimitConfig{}, false
	}
	return k.rateLimiter, k.rateLimit, true
}

func (k *RuntimeKernel) Orchestrator() *Orchestrator {
	if k == nil || k.orchestrator == nil {
		return nil
	}
	return k.orchestrator
}

func (k *RuntimeKernel) Close() {
	if k == nil {
		return
	}
	if k.cancelOutboxPoller != nil {
		k.cancelOutboxPoller()
		k.cancelOutboxPoller = nil
	}
	k.rateLimiter = nil
	k.rateLimit = RateLimitConfig{}
}

func normalizeRateLimitConfig(cfg RateLimitConfig) RateLimitConfig {
	if !cfg.Enabled {
		return RateLimitConfig{}
	}
	if cfg.DefaultLimit <= 0 {
		cfg.DefaultLimit = 1
	}
	if cfg.DefaultWindow <= 0 {
		cfg.DefaultWindow = defaultRateLimitWindow
	}
	return cfg
}
