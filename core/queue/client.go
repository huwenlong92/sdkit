package queue

import "context"

type Client interface {
	Enqueue(ctx context.Context, task Task, opts ...Option) (*TaskInfo, error)
	BatchEnqueue(ctx context.Context, tasks []Task, opts ...Option) ([]*TaskInfo, error)
	Close() error
}

type HandlerFunc func(context.Context, *Message) error

type Middleware func(HandlerFunc) HandlerFunc

type Worker interface {
	Handle(taskType string, handler HandlerFunc)
	Use(middlewares ...Middleware)
	Run(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

type Manager interface {
	Supports(cap Capability) bool
	Capabilities() map[Capability]bool

	ListQueues(ctx context.Context) ([]*QueueInfo, error)
	GetQueue(ctx context.Context, queue string) (*QueueInfo, error)

	ListTasks(ctx context.Context, query TaskQuery) ([]*TaskInfo, error)
	GetTask(ctx context.Context, queue, taskID string) (*TaskInfo, error)

	DeleteTask(ctx context.Context, queue, taskID string) error
	RetryTask(ctx context.Context, queue, taskID string) error
	ArchiveTask(ctx context.Context, queue, taskID string) error
	CancelTask(ctx context.Context, queue, taskID string) error

	PauseQueue(ctx context.Context, queue string) error
	ResumeQueue(ctx context.Context, queue string) error
}

type QueueRunner interface {
	Client
	Worker
	Manager
}
