package event

import "github.com/huwenlong92/sdkit/core/queue"

type RuntimeEvent = queue.RuntimeEvent
type RuntimeEventType = queue.RuntimeEventType
type Publisher = queue.EventPublisher

const (
	TaskStarted    = queue.RuntimeEventTaskStarted
	TaskSuccess    = queue.RuntimeEventTaskSuccess
	TaskFailed     = queue.RuntimeEventTaskFailed
	TaskRetry      = queue.RuntimeEventTaskRetry
	TaskDeadLetter = queue.RuntimeEventTaskDeadLetter
	TaskTimeout    = queue.RuntimeEventTaskTimeout
)
