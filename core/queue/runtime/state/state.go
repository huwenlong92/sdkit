package state

import "github.com/huwenlong92/sdkit/core/queue"

type TaskState = queue.TaskState

const (
	TaskPending    = queue.TaskPending
	TaskRunning    = queue.TaskRunning
	TaskRetrying   = queue.TaskRetrying
	TaskFailed     = queue.TaskFailed
	TaskDeadLetter = queue.TaskDeadLetter
	TaskSuccess    = queue.TaskSuccess
)

var SetTaskState = queue.SetTaskState
var CanTransitionTaskState = queue.CanTransitionTaskState
var TransitionTaskState = queue.TransitionTaskState
