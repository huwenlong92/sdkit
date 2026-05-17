package crontab

import (
	"context"
	"sync"
	"time"
)

type BatchLogger struct {
	writer        LogWriter
	ch            chan RunLog
	batchSize     int
	flushInterval time.Duration
	stop          chan struct{}
	wg            sync.WaitGroup
}

func NewBatchLogger(writer LogWriter, batchSize int, flushInterval time.Duration) *BatchLogger {
	if batchSize <= 0 {
		batchSize = 100
	}
	if flushInterval <= 0 {
		flushInterval = 3 * time.Second
	}
	return &BatchLogger{
		writer:        writer,
		ch:            make(chan RunLog, batchSize*2),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		stop:          make(chan struct{}),
	}
}

func (b *BatchLogger) Start(ctx context.Context) {
	b.wg.Add(1)
	go b.loop(ctx)
}

func (b *BatchLogger) Stop(ctx context.Context) error {
	close(b.stop)
	b.wg.Wait()
	return nil
}

func (b *BatchLogger) Write(ctx context.Context, log RunLog) error {
	select {
	case b.ch <- log:
		return nil
	default:
		return b.writer.Write(ctx, log)
	}
}

func (b *BatchLogger) WriteBatch(ctx context.Context, logs []RunLog) error {
	return b.writer.WriteBatch(ctx, logs)
}

func (b *BatchLogger) loop(ctx context.Context) {
	defer b.wg.Done()

	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	var batch []RunLog

	flush := func() {
		if len(batch) == 0 {
			return
		}
		_ = b.writer.WriteBatch(ctx, batch)
		batch = nil
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case <-b.stop:
			flush()
			return
		case log := <-b.ch:
			batch = append(batch, log)
			if len(batch) >= b.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
