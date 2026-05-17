package queue

type TaskState string

const (
	TaskPending    TaskState = "pending"
	TaskRunning    TaskState = "running"
	TaskRetrying   TaskState = "retrying"
	TaskFailed     TaskState = "failed"
	TaskDeadLetter TaskState = "deadletter"
	TaskSuccess    TaskState = "success"

	StatePending   TaskState = "pending"
	StateActive    TaskState = "active"
	StateScheduled TaskState = "scheduled"
	StateRetry     TaskState = "retry"
	StateSucceeded TaskState = "succeeded"
	StateFailed    TaskState = "failed"
	StateArchived  TaskState = "archived"
	StateCanceled  TaskState = "canceled"
	StateUnknown   TaskState = "unknown"
)

// SetTaskState 按状态机规则推进任务状态。非法流转会被忽略。
//
// 新代码优先使用 TransitionTaskState 获取是否流转成功。
func SetTaskState(msg *Message, state TaskState) {
	_ = TransitionTaskState(msg, state)
}

func setTaskState(msg *Message, state TaskState) {
	if msg == nil || state == "" {
		return
	}
	msg.State = state
	if msg.Runtime == nil {
		msg.Runtime = RuntimeContextFromMessage(msg)
	}
	msg.Runtime.TaskState = state
}

func CanTransitionTaskState(from TaskState, to TaskState) bool {
	if to == "" {
		return false
	}
	if from == "" {
		return to == TaskPending || to == TaskRunning
	}
	if from == to {
		return true
	}
	switch from {
	case TaskPending:
		return to == TaskRunning
	case TaskRunning:
		return to == TaskSuccess || to == TaskRetrying || to == TaskFailed
	case TaskRetrying:
		return to == TaskRunning
	case TaskFailed:
		return to == TaskDeadLetter
	default:
		return false
	}
}

func TransitionTaskState(msg *Message, next TaskState) bool {
	if msg == nil {
		return false
	}
	if !CanTransitionTaskState(msg.State, next) {
		return false
	}
	setTaskState(msg, next)
	return true
}

type QueueState string

const (
	QueueRunning  QueueState = "running"
	QueuePaused   QueueState = "paused"
	QueueDraining QueueState = "draining"
	QueueStopped  QueueState = "stopped"
	QueueFailed   QueueState = "failed"
	QueueUnknown  QueueState = "unknown"
)
