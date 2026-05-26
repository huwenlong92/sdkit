package queue_test

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/queue"
)

type runtimeRunner struct {
	handlers   map[string]queue.HandlerFunc
	listQueues int
	ran        bool
	shutdown   bool
}

func (r *runtimeRunner) Enqueue(context.Context, queue.Task, ...queue.Option) (*queue.TaskInfo, error) {
	return &queue.TaskInfo{ID: "task-1"}, nil
}

func (r *runtimeRunner) BatchEnqueue(context.Context, []queue.Task, ...queue.Option) ([]*queue.TaskInfo, error) {
	return []*queue.TaskInfo{{ID: "task-1"}}, nil
}

func (r *runtimeRunner) Close() error { return nil }

func (r *runtimeRunner) Handle(pattern string, handler queue.HandlerFunc) {
	if r.handlers == nil {
		r.handlers = map[string]queue.HandlerFunc{}
	}
	r.handlers[pattern] = handler
}

func (r *runtimeRunner) Use(...queue.Middleware) {}

func (r *runtimeRunner) Run(ctx context.Context) error {
	r.ran = true
	if queue.Runtime(ctx) == nil {
		return errors.New("runtime missing from context")
	}
	return nil
}

func (r *runtimeRunner) Shutdown(ctx context.Context) error {
	r.shutdown = queue.Runtime(ctx) != nil
	return nil
}

func (r *runtimeRunner) Supports(cap queue.Capability) bool {
	return r.Capabilities()[cap]
}

func (r *runtimeRunner) Capabilities() map[queue.Capability]bool {
	return map[queue.Capability]bool{queue.CapInspector: true}
}

func (r *runtimeRunner) ListQueues(context.Context) ([]*queue.QueueInfo, error) {
	r.listQueues++
	return []*queue.QueueInfo{{Name: queue.DefaultQueueName}}, nil
}

func (r *runtimeRunner) GetQueue(context.Context, string) (*queue.QueueInfo, error) {
	return &queue.QueueInfo{Name: queue.DefaultQueueName}, nil
}

func (r *runtimeRunner) ListTasks(context.Context, queue.TaskQuery) ([]*queue.TaskInfo, error) {
	return nil, nil
}

func (r *runtimeRunner) GetTask(context.Context, string, string) (*queue.TaskInfo, error) {
	return nil, nil
}

func (r *runtimeRunner) DeleteTask(context.Context, string, string) error { return nil }
func (r *runtimeRunner) RetryTask(context.Context, string, string) error  { return nil }
func (r *runtimeRunner) ArchiveTask(context.Context, string, string) error {
	return nil
}
func (r *runtimeRunner) CancelTask(context.Context, string, string) error { return nil }
func (r *runtimeRunner) PauseQueue(context.Context, string) error         { return nil }
func (r *runtimeRunner) ResumeQueue(context.Context, string) error        { return nil }

func TestRuntimeInstanceExposesRuntimeAndOperations(t *testing.T) {
	runner := &runtimeRunner{}
	runtime := queue.NewRuntimeInstance(
		runner,
		queue.WithRuntimeMetadata(queue.RuntimeMetadata{
			Name:   "worker-1",
			Driver: "memory",
			Queues: map[string]int{queue.DefaultQueueName: 1},
		}),
	)

	if queue.From(runtime) != runtime {
		t.Fatal("From(runtime) did not return runtime instance")
	}
	if queue.Runtime(queue.ContextWithRuntime(context.Background(), runtime)) != runtime {
		t.Fatal("Runtime(ctx) did not return runtime instance")
	}
	metadata := runtime.Metadata()
	metadata.Queues[queue.DefaultQueueName] = 9
	if runtime.Metadata().Queues[queue.DefaultQueueName] != 1 {
		t.Fatal("runtime metadata was not cloned")
	}

	queues, err := runtime.Operations().Status(context.Background())
	if err != nil {
		t.Fatalf("operations status: %v", err)
	}
	if len(queues) != 1 || queues[0].Name != queue.DefaultQueueName || runner.listQueues != 1 {
		t.Fatalf("operations status = %#v, list calls=%d", queues, runner.listQueues)
	}
	if err := runtime.Run(context.Background()); err != nil {
		t.Fatalf("runtime run: %v", err)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("runtime shutdown: %v", err)
	}
	if !runner.ran || !runner.shutdown {
		t.Fatalf("runner lifecycle ran=%v shutdown=%v", runner.ran, runner.shutdown)
	}
}

func TestRegistryRuntimeRegistersAndDispatches(t *testing.T) {
	runner := &runtimeRunner{}
	runtime := queue.NewRuntimeInstance(runner)
	registry := runtime.NewRegistry()
	calls := 0

	registry.Use(func(next queue.HandlerFunc) queue.HandlerFunc {
		return func(ctx context.Context, msg *queue.Message) error {
			calls++
			return next(ctx, msg)
		}
	})
	if err := registry.Register("user.sync", func(ctx context.Context, msg *queue.Message) error {
		calls++
		if _, ok := queue.MessageFromContext(ctx); !ok {
			t.Fatal("message missing from context")
		}
		return nil
	}); err != nil {
		t.Fatalf("registry register: %v", err)
	}

	handler := runner.handlers["user.sync"]
	if handler == nil {
		t.Fatal("worker handler was not bound")
	}
	if err := handler(context.Background(), &queue.Message{ID: "1", Type: "user.sync"}); err != nil {
		t.Fatalf("handler dispatch: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	entries := runtime.Registry().Entries()
	if len(entries) != 1 || entries[0].Pattern != "user.sync" || entries[0].MiddlewareCount != 0 {
		t.Fatalf("entries = %#v", entries)
	}
}
