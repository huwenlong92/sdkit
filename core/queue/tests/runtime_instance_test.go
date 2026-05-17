package tests

import (
	"context"
	"errors"
	"testing"

	corequeue "github.com/huwenlong92/sdkit/core/queue"
)

type runtimeRunner struct {
	handlers   map[string]corequeue.HandlerFunc
	listQueues int
	ran        bool
	shutdown   bool
}

func (r *runtimeRunner) Enqueue(context.Context, corequeue.Task, ...corequeue.Option) (*corequeue.TaskInfo, error) {
	return &corequeue.TaskInfo{ID: "task-1"}, nil
}

func (r *runtimeRunner) BatchEnqueue(context.Context, []corequeue.Task, ...corequeue.Option) ([]*corequeue.TaskInfo, error) {
	return []*corequeue.TaskInfo{{ID: "task-1"}}, nil
}

func (r *runtimeRunner) Close() error { return nil }

func (r *runtimeRunner) Handle(pattern string, handler corequeue.HandlerFunc) {
	if r.handlers == nil {
		r.handlers = map[string]corequeue.HandlerFunc{}
	}
	r.handlers[pattern] = handler
}

func (r *runtimeRunner) Use(...corequeue.Middleware) {}

func (r *runtimeRunner) Run(ctx context.Context) error {
	r.ran = true
	if corequeue.Runtime(ctx) == nil {
		return errors.New("runtime missing from context")
	}
	return nil
}

func (r *runtimeRunner) Shutdown(ctx context.Context) error {
	r.shutdown = corequeue.Runtime(ctx) != nil
	return nil
}

func (r *runtimeRunner) Supports(cap corequeue.Capability) bool {
	return r.Capabilities()[cap]
}

func (r *runtimeRunner) Capabilities() map[corequeue.Capability]bool {
	return map[corequeue.Capability]bool{corequeue.CapInspector: true}
}

func (r *runtimeRunner) ListQueues(context.Context) ([]*corequeue.QueueInfo, error) {
	r.listQueues++
	return []*corequeue.QueueInfo{{Name: corequeue.DefaultQueueName}}, nil
}

func (r *runtimeRunner) GetQueue(context.Context, string) (*corequeue.QueueInfo, error) {
	return &corequeue.QueueInfo{Name: corequeue.DefaultQueueName}, nil
}

func (r *runtimeRunner) ListTasks(context.Context, corequeue.TaskQuery) ([]*corequeue.TaskInfo, error) {
	return nil, nil
}

func (r *runtimeRunner) GetTask(context.Context, string, string) (*corequeue.TaskInfo, error) {
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
	runtime := corequeue.NewRuntimeInstance(
		runner,
		corequeue.WithRuntimeMetadata(corequeue.RuntimeMetadata{
			Name:   "worker-1",
			Driver: "memory",
			Queues: map[string]int{corequeue.DefaultQueueName: 1},
		}),
	)

	if corequeue.From(runtime) != runtime {
		t.Fatal("From(runtime) did not return runtime instance")
	}
	if corequeue.Runtime(corequeue.ContextWithRuntime(context.Background(), runtime)) != runtime {
		t.Fatal("Runtime(ctx) did not return runtime instance")
	}
	metadata := runtime.Metadata()
	metadata.Queues[corequeue.DefaultQueueName] = 9
	if runtime.Metadata().Queues[corequeue.DefaultQueueName] != 1 {
		t.Fatal("runtime metadata was not cloned")
	}

	queues, err := runtime.Operations().Status(context.Background())
	if err != nil {
		t.Fatalf("operations status: %v", err)
	}
	if len(queues) != 1 || queues[0].Name != corequeue.DefaultQueueName || runner.listQueues != 1 {
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
	runtime := corequeue.NewRuntimeInstance(runner)
	registry := runtime.NewRegistry()
	calls := 0

	registry.Use(func(next corequeue.HandlerFunc) corequeue.HandlerFunc {
		return func(ctx context.Context, msg *corequeue.Message) error {
			calls++
			return next(ctx, msg)
		}
	})
	if err := registry.Register("user.sync", func(ctx context.Context, msg *corequeue.Message) error {
		calls++
		if _, ok := corequeue.MessageFromContext(ctx); !ok {
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
	if err := handler(context.Background(), &corequeue.Message{ID: "1", Type: "user.sync"}); err != nil {
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
