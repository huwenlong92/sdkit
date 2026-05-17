package accesslog

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	defaultQueueSize     = 1024
	defaultBatchSize     = 100
	defaultFlushInterval = 200 * time.Millisecond
)

type Config struct {
	QueueSize     int
	BatchSize     int
	FlushInterval time.Duration
}

// Logger asynchronously buffers access log entries and writes them in batches.
type Logger struct {
	writer        Writer
	ch            chan *Entry
	batchSize     int
	flushInterval time.Duration
	once          sync.Once
}

func NewLogger(writer Writer, cfg Config) *Logger {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = defaultQueueSize
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = defaultFlushInterval
	}
	return &Logger{
		writer:        writer,
		ch:            make(chan *Entry, cfg.QueueSize),
		batchSize:     cfg.BatchSize,
		flushInterval: cfg.FlushInterval,
	}
}

// Push enqueues an entry without blocking the request path.
// If the queue is full, the entry is dropped.
func (l *Logger) Push(entry *Entry) {
	if l == nil || entry == nil {
		return
	}
	select {
	case l.ch <- entry:
	default:
		log.Printf("accesslog queue full, dropping log: method=%s path=%s", entry.Method, entry.Path)
	}
}

// Start consumes queued entries until ctx is done, then flushes remaining logs.
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

	batch := make([]*Entry, 0, l.batchSize)

	flush := func(flushCtx context.Context) {
		if len(batch) == 0 {
			return
		}
		entries := make([]*Entry, len(batch))
		copy(entries, batch)
		batch = batch[:0]
		if err := l.writer.WriteBatch(flushCtx, entries); err != nil {
			log.Printf("accesslog write batch failed: count=%d err=%v", len(entries), err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case entry := <-l.ch:
					batch = append(batch, entry)
				default:
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					flush(shutdownCtx)
					cancel()
					return
				}
			}
		case entry := <-l.ch:
			batch = append(batch, entry)
			if len(batch) >= l.batchSize {
				flush(ctx)
			}
		case <-ticker.C:
			flush(ctx)
		}
	}
}
