package queue

type OutboxTask struct {
	Task    Task
	Options []Option
}

func NewOutboxTask(task Task, opts ...Option) OutboxTask {
	return OutboxTask{Task: task, Options: opts}
}
