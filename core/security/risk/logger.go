package risk

import (
	"context"
	"sync"
	"time"

	"github.com/huwenlong92/sdkit/core/logger"
	"go.uber.org/zap"
)

const (
	defaultLoggerQueueSize     = 2048
	defaultLoggerBatchSize     = 100
	defaultLoggerFlushInterval = 200 * time.Millisecond
)

type DecisionRecord struct {
	Event    Event
	Decision Decision
}

type DecisionWriter interface {
	WriteDecisionBatch(ctx context.Context, records []DecisionRecord) error
}

type LoggerConfig struct {
	QueueSize     int
	BatchSize     int
	FlushInterval time.Duration
}

type Logger struct {
	writer        DecisionWriter
	ch            chan DecisionRecord
	batchSize     int
	flushInterval time.Duration
	once          sync.Once
}

func NewLogger(writer DecisionWriter, cfg LoggerConfig) *Logger {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = defaultLoggerQueueSize
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultLoggerBatchSize
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = defaultLoggerFlushInterval
	}
	return &Logger{
		writer:        writer,
		ch:            make(chan DecisionRecord, cfg.QueueSize),
		batchSize:     cfg.BatchSize,
		flushInterval: cfg.FlushInterval,
	}
}

func (l *Logger) Push(event Event, decision *Decision) bool {
	if l == nil || decision == nil {
		return false
	}
	record := NewDecisionRecord(event, decision)
	select {
	case l.ch <- record:
		return true
	default:
		return false
	}
}

func (l *Logger) Start(ctx context.Context) {
	if l == nil || l.writer == nil {
		return
	}
	l.once.Do(func() {
		go l.run(ctx)
	})
}

func (l *Logger) run(ctx context.Context) {
	ticker := time.NewTicker(l.flushInterval)
	defer ticker.Stop()

	batch := make([]DecisionRecord, 0, l.batchSize)
	flush := func(flushCtx context.Context) {
		if len(batch) == 0 {
			return
		}
		records := make([]DecisionRecord, len(batch))
		copy(records, batch)
		batch = batch[:0]
		if err := l.writer.WriteDecisionBatch(flushCtx, records); err != nil {
			logDecisionWriteError(flushCtx, len(records), err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case record := <-l.ch:
					batch = append(batch, record)
				default:
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					flush(shutdownCtx)
					cancel()
					return
				}
			}
		case record := <-l.ch:
			batch = append(batch, record)
			if len(batch) >= l.batchSize {
				flush(ctx)
			}
		case <-ticker.C:
			flush(ctx)
		}
	}
}

func NewDecisionRecord(event Event, decision *Decision) DecisionRecord {
	if decision == nil {
		return DecisionRecord{Event: cloneEvent(event)}
	}
	return DecisionRecord{
		Event:    cloneEvent(event),
		Decision: cloneDecision(*decision),
	}
}

func cloneEvent(event Event) Event {
	event.Extra = cloneMap(event.Extra)
	return event
}

func cloneDecision(decision Decision) Decision {
	decision.Reasons = cloneMapSlice(decision.Reasons)
	decision.Hits = cloneHitDecisions(decision.Hits)
	return decision
}

func cloneHitDecisions(hits []HitDecision) []HitDecision {
	if len(hits) == 0 {
		return nil
	}
	out := make([]HitDecision, len(hits))
	for i := range hits {
		out[i] = hits[i]
		out[i].Reason = cloneMap(hits[i].Reason)
		out[i].Snapshot = cloneMap(hits[i].Snapshot)
	}
	return out
}

func cloneMapSlice(values []map[string]any) []map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make([]map[string]any, len(values))
	for i := range values {
		out[i] = cloneMap(values[i])
	}
	return out
}

func cloneMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func logDecisionWriteError(ctx context.Context, count int, err error) {
	fields := logger.ContextFields(ctx)
	if !hasLogField(fields, logger.TraceIDKey) {
		fields = append([]zap.Field{zap.String(logger.TraceIDKey, "")}, fields...)
	}
	fields = append(fields, zap.Int("count", count), zap.Error(err))
	logger.Named("security-risk").Error("risk decision write batch failed", fields...)
}

func hasLogField(fields []zap.Field, key string) bool {
	for _, field := range fields {
		if field.Key == key {
			return true
		}
	}
	return false
}
