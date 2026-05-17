package queue

import (
	"context"
	"time"
)

type RuntimeEventType string

const (
	RuntimeEventTaskStarted    RuntimeEventType = "task.started"
	RuntimeEventTaskSuccess    RuntimeEventType = "task.success"
	RuntimeEventTaskFailed     RuntimeEventType = "task.failed"
	RuntimeEventTaskRetry      RuntimeEventType = "task.retry"
	RuntimeEventTaskDeadLetter RuntimeEventType = "task.deadletter"
	RuntimeEventTaskTimeout    RuntimeEventType = "task.timeout"
)

type RuntimeEvent struct {
	Type      RuntimeEventType
	Message   *Message
	Error     error
	Timestamp time.Time
}

type EventPublisher interface {
	Publish(ctx context.Context, event RuntimeEvent)
}
