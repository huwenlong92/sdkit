package queue

import (
	"context"
	"sync"
	"time"

	"github.com/huwenlong92/sdkit/core/logger"

	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	defaultFailureQueueSize     = 1024
	defaultFailureBatchSize     = 100
	defaultFailureFlushInterval = 200 * time.Millisecond
)

type FailureLogConfig struct {
	QueueSize     int
	BatchSize     int
	FlushInterval time.Duration
}

type FailureWriter interface {
	WriteBatch(ctx context.Context, failures []*Failure) error
}

type FailureLogger struct {
	writer        FailureWriter
	ch            chan failureLogItem
	batchSize     int
	flushInterval time.Duration
	once          sync.Once
}

type failureLogItem struct {
	ctx     context.Context
	failure *Failure
}

func NewFailureLogger(writer FailureWriter, cfg FailureLogConfig) *FailureLogger {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = defaultFailureQueueSize
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultFailureBatchSize
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = defaultFailureFlushInterval
	}
	return &FailureLogger{
		writer:        writer,
		ch:            make(chan failureLogItem, cfg.QueueSize),
		batchSize:     cfg.BatchSize,
		flushInterval: cfg.FlushInterval,
	}
}

func (l *FailureLogger) Push(failure *Failure) {
	l.PushContext(context.Background(), failure)
}

func (l *FailureLogger) PushContext(ctx context.Context, failure *Failure) {
	if l == nil || failure == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithoutCancel(ctx)
	select {
	case l.ch <- failureLogItem{ctx: ctx, failure: cloneFailure(failure)}:
	default:
		logger.L.Warn("队列失败日志缓冲已满，丢弃日志",
			zap.String("queue", failure.Queue),
			zap.String("type", failure.Type),
			zap.String("task_id", failure.TaskID),
		)
	}
}

func (l *FailureLogger) Handler() FailureHandler {
	return func(ctx context.Context, failure *Failure) {
		l.PushContext(ctx, failure)
	}
}

func (l *FailureLogger) Start(ctx context.Context) {
	if l == nil || l.writer == nil {
		return
	}
	l.once.Do(func() {
		go l.run(ctx)
	})
}

func (l *FailureLogger) run(ctx context.Context) {
	ticker := time.NewTicker(l.flushInterval)
	defer ticker.Stop()

	batch := make([]failureLogItem, 0, l.batchSize)

	flush := func(flushCtx context.Context) {
		if len(batch) == 0 {
			return
		}
		items := make([]failureLogItem, len(batch))
		copy(items, batch)
		batch = batch[:0]
		for _, group := range failureLogGroups(flushCtx, items) {
			if err := l.writer.WriteBatch(group.ctx, group.failures); err != nil {
				logger.L.Error("队列失败日志批量写入失败", zap.Int("count", len(group.failures)), zap.Error(err))
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case item := <-l.ch:
					batch = append(batch, item)
				default:
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					flush(shutdownCtx)
					cancel()
					return
				}
			}
		case item := <-l.ch:
			batch = append(batch, item)
			if len(batch) >= l.batchSize {
				flush(ctx)
			}
		case <-ticker.C:
			flush(ctx)
		}
	}
}

type failureLogGroup struct {
	ctx      context.Context
	failures []*Failure
}

func failureLogGroups(defaultCtx context.Context, items []failureLogItem) []failureLogGroup {
	if defaultCtx == nil {
		defaultCtx = context.Background()
	}
	groups := make([]failureLogGroup, 0, len(items))
	index := make(map[string]int, len(items))
	for _, item := range items {
		if item.failure == nil {
			continue
		}
		ctx := item.ctx
		key := spanGroupKey(ctx)
		if key == "" {
			ctx = defaultCtx
			key = "__default__"
		}
		i, ok := index[key]
		if !ok {
			index[key] = len(groups)
			groups = append(groups, failureLogGroup{ctx: ctx})
			i = len(groups) - 1
		}
		groups[i].failures = append(groups[i].failures, item.failure)
	}
	return groups
}

func spanGroupKey(ctx context.Context) string {
	spanContext := oteltrace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return ""
	}
	return spanContext.TraceID().String() + "/" + spanContext.SpanID().String()
}

func cloneFailure(failure *Failure) *Failure {
	if failure == nil {
		return nil
	}
	out := *failure
	if failure.Payload != nil {
		out.Payload = make([]byte, len(failure.Payload))
		copy(out.Payload, failure.Payload)
	}
	return &out
}
