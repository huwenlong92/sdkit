package queue

import (
	"context"
	"time"
)

type Locker interface {
	Lock(ctx context.Context, key string, ttl time.Duration) (unlock func(context.Context) error, ok bool, err error)
}

type Idempotency interface {
	Done(ctx context.Context, key string) (bool, error)
	MarkDone(ctx context.Context, key string, ttl time.Duration) error
}

type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error)
}

type MetricsLabels struct {
	Queue    string
	TaskType string
	Status   string
	Worker   string
}

type MetricsRecorder interface {
	IncCounter(ctx context.Context, name string, labels MetricsLabels, value int64)
	ObserveDuration(ctx context.Context, name string, labels MetricsLabels, value time.Duration)
}

type RetryStrategy interface {
	NextRetry(ctx context.Context, msg *Message, retryCount int, err error) (time.Duration, bool)
}

type RetryStrategyFunc func(ctx context.Context, msg *Message, retryCount int, err error) (time.Duration, bool)

func (fn RetryStrategyFunc) NextRetry(ctx context.Context, msg *Message, retryCount int, err error) (time.Duration, bool) {
	if fn == nil {
		return 0, false
	}
	return fn(ctx, msg, retryCount, err)
}

func RetryDelayStrategy(fn RetryDelayFunc) RetryStrategy {
	if fn == nil {
		return nil
	}
	return RetryStrategyFunc(func(_ context.Context, msg *Message, retryCount int, err error) (time.Duration, bool) {
		return fn(retryCount, err, msg), true
	})
}

type DeadLetter interface {
	Push(ctx context.Context, msg *Message, err error) error
}

type ConcurrencyLimiter interface {
	Acquire(ctx context.Context, key string) error
	Release(ctx context.Context, key string)
}

type BackoffStrategy interface {
	NextDelay(retry int, err error) time.Duration
}

type ProgressReporter interface {
	Report(ctx context.Context, taskID string, percent int, message string) error
}

type WorkerHeartbeat struct {
	WorkerName      string
	Hostname        string
	PID             int
	Queues          map[string]int
	Concurrency     int
	StartedAt       time.Time
	LastHeartbeatAt time.Time
}

type Outbox interface {
	Save(ctx context.Context, task Task, opts ...Option) error
	Flush(ctx context.Context, limit int) error
}

type AuditLogger interface {
	LogQueueAction(ctx context.Context, action QueueAuditAction) error
}

type QueueAuditAction struct {
	OperatorID      string
	OperatorName    string
	Action          string
	Queue           string
	TaskID          string
	TaskType        string
	BeforeState     TaskState
	AfterState      TaskState
	PayloadSnapshot string
	CreatedAt       time.Time
}
