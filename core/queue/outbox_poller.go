package queue

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/logger"

	"go.uber.org/zap"
)

const (
	defaultOutboxBatchSize     = 100
	defaultOutboxFlushInterval = 5 * time.Second
)

type OutboxPollerConfig struct {
	BatchSize     int
	FlushInterval time.Duration
}

type OutboxPoller struct {
	outbox Outbox
	cfg    OutboxPollerConfig
}

func NewOutboxPoller(outbox Outbox, cfg OutboxPollerConfig) *OutboxPoller {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultOutboxBatchSize
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = defaultOutboxFlushInterval
	}
	return &OutboxPoller{outbox: outbox, cfg: cfg}
}

func (p *OutboxPoller) Run(ctx context.Context) error {
	if p == nil || p.outbox == nil {
		return ErrNotInitialized
	}
	if err := p.flush(ctx); err != nil {
		logger.WithContext(ctx, logger.L).Warn("Queue Outbox刷新失败", zap.Error(err))
	}
	ticker := time.NewTicker(p.cfg.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := p.flush(ctx); err != nil {
				logger.WithContext(ctx, logger.L).Warn("Queue Outbox刷新失败", zap.Error(err))
			}
		}
	}
}

func (p *OutboxPoller) Start(ctx context.Context) context.CancelFunc {
	runCtx, cancel := context.WithCancel(ctx)
	go func() {
		if err := p.Run(runCtx); err != nil && runCtx.Err() == nil {
			logger.WithContext(runCtx, logger.L).Warn("Queue Outbox Poller退出", zap.Error(err))
		}
	}()
	return cancel
}

func (p *OutboxPoller) flush(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.outbox.Flush(ctx, p.cfg.BatchSize)
}
