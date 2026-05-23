package queue_test

import (
	"context"
	"errors"
	"testing"
	"time"

	corequeue "github.com/huwenlong92/sdkit/core/queue"
)

func TestRegistryRuntimeMetadataAndContextMetadata(t *testing.T) {
	runner := &runtimeRunner{}
	runtime := corequeue.NewRuntimeInstance(runner)
	registry := runtime.NewRegistry()
	seenMetadata := false

	registry.Use(func(next corequeue.HandlerFunc) corequeue.HandlerFunc {
		return func(ctx context.Context, msg *corequeue.Message) error {
			if corequeue.Runtime(ctx) != runtime {
				t.Fatal("runtime missing from handler context")
			}
			metadata, ok := corequeue.MetadataFromContext(ctx)
			if !ok || metadata.TaskID != "task-1" || metadata.Queue != "critical" || metadata.RetryCount != 2 {
				t.Fatalf("context metadata = %#v, ok=%v", metadata, ok)
			}
			seenMetadata = true
			return next(ctx, msg)
		}
	})

	if err := registry.Register(
		"user.sync",
		func(ctx context.Context, msg *corequeue.Message) error { return nil },
		corequeue.WithRetry(3),
		corequeue.WithQueue("critical"),
		corequeue.WithTimeout(time.Second),
		corequeue.WithDelay(2*time.Second),
		corequeue.WithPriority(7),
		corequeue.WithTrace(false),
	); err != nil {
		t.Fatalf("registry register: %v", err)
	}

	metadata := registry.Metadata()
	if metadata.Middleware.Count != 1 {
		t.Fatalf("middleware metadata = %#v", metadata.Middleware)
	}
	if len(metadata.Handlers) != 1 {
		t.Fatalf("handlers = %#v", metadata.Handlers)
	}
	handler := metadata.Handlers[0]
	if handler.Queue != "critical" || handler.MaxRetry == nil || *handler.MaxRetry != 3 {
		t.Fatalf("handler retry/queue metadata = %#v", handler)
	}
	if handler.Timeout != time.Second || handler.Delay != 2*time.Second || handler.Priority != 7 || handler.Trace {
		t.Fatalf("handler runtime metadata = %#v", handler)
	}

	bound := runner.handlers["user.sync"]
	if bound == nil {
		t.Fatal("handler was not bound")
	}
	if err := bound(context.Background(), &corequeue.Message{
		ID:         "task-1",
		Type:       "user.sync",
		Queue:      "critical",
		RetryCount: 2,
		MaxRetry:   3,
	}); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !seenMetadata {
		t.Fatal("middleware did not see context metadata")
	}
}

type operationsManager struct {
	queues       []*corequeue.QueueInfo
	tasks        []*corequeue.TaskInfo
	deleted      []string
	paused       []string
	resumed      []string
	failedAsArch bool
}

func (m *operationsManager) Supports(cap corequeue.Capability) bool { return m.Capabilities()[cap] }

func (m *operationsManager) Capabilities() map[corequeue.Capability]bool {
	return map[corequeue.Capability]bool{corequeue.CapInspector: true}
}

func (m *operationsManager) ListQueues(context.Context) ([]*corequeue.QueueInfo, error) {
	return m.queues, nil
}

func (m *operationsManager) GetQueue(_ context.Context, queueName string) (*corequeue.QueueInfo, error) {
	for _, queue := range m.queues {
		if queue != nil && queue.Name == queueName {
			return queue, nil
		}
	}
	return nil, corequeue.ErrQueueNotFound
}

func (m *operationsManager) ListTasks(_ context.Context, query corequeue.TaskQuery) ([]*corequeue.TaskInfo, error) {
	if query.State == corequeue.StateFailed && m.failedAsArch {
		return nil, corequeue.ErrCapabilityUnsupported
	}
	return m.tasks, nil
}

func (m *operationsManager) GetTask(context.Context, string, string) (*corequeue.TaskInfo, error) {
	return nil, nil
}

func (m *operationsManager) DeleteTask(_ context.Context, queueName string, taskID string) error {
	m.deleted = append(m.deleted, queueName+":"+taskID)
	return nil
}

func (m *operationsManager) RetryTask(context.Context, string, string) error { return nil }

func (m *operationsManager) ArchiveTask(context.Context, string, string) error { return nil }

func (m *operationsManager) CancelTask(context.Context, string, string) error { return nil }

func (m *operationsManager) PauseQueue(_ context.Context, queueName string) error {
	m.paused = append(m.paused, queueName)
	return nil
}

func (m *operationsManager) ResumeQueue(_ context.Context, queueName string) error {
	m.resumed = append(m.resumed, queueName)
	return nil
}

func TestOperationsRuntimeStatusMetricsAndMaintenance(t *testing.T) {
	manager := &operationsManager{
		queues: []*corequeue.QueueInfo{{
			Name:      corequeue.DefaultQueueName,
			State:     corequeue.QueuePaused,
			Pending:   2,
			Active:    1,
			Failed:    3,
			Processed: 5,
			FailedAll: 3,
		}},
		tasks: []*corequeue.TaskInfo{{ID: "task-1", Queue: corequeue.DefaultQueueName}},
	}
	operations := corequeue.NewOperationsRuntime(manager)
	operations.SetMetadata(corequeue.RuntimeMetadata{
		Name:        "worker-main",
		Service:     "worker",
		Worker:      "default",
		Queues:      map[string]int{corequeue.DefaultQueueName: 1},
		Concurrency: 4,
	})

	metrics, err := operations.Metrics(context.Background())
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if metrics.Total.Pending != 2 || metrics.Total.Failed != 3 || metrics.Total.Processed != 5 {
		t.Fatalf("metrics = %#v", metrics.Total)
	}

	status, err := operations.RuntimeStatus(context.Background())
	if err != nil {
		t.Fatalf("runtime status: %v", err)
	}
	if status.State != corequeue.RuntimePaused || status.Worker.Name != "default" || status.Worker.Concurrency != 4 {
		t.Fatalf("status = %#v", status)
	}

	if err := operations.DrainQueue(context.Background(), corequeue.DefaultQueueName); err != nil {
		t.Fatalf("drain: %v", err)
	}
	status, err = operations.RuntimeStatus(context.Background())
	if err != nil {
		t.Fatalf("runtime status after drain: %v", err)
	}
	if status.State != corequeue.RuntimeDraining || len(manager.paused) != 1 {
		t.Fatalf("drain status = %#v paused=%v", status, manager.paused)
	}
	if err := operations.ResumeQueue(context.Background(), corequeue.DefaultQueueName); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if len(manager.resumed) != 1 {
		t.Fatalf("resume calls = %v", manager.resumed)
	}

	cleaned, err := operations.CleanTasks(context.Background(), corequeue.TaskQuery{Queue: corequeue.DefaultQueueName})
	if err != nil {
		t.Fatalf("clean: %v", err)
	}
	if cleaned != 1 || len(manager.deleted) != 1 || manager.deleted[0] != "default:task-1" {
		t.Fatalf("cleaned=%d deleted=%v", cleaned, manager.deleted)
	}
}

func TestOperationsRuntimeFailedTasksFallback(t *testing.T) {
	manager := &operationsManager{
		failedAsArch: true,
		tasks:        []*corequeue.TaskInfo{{ID: "archived-1", State: corequeue.StateArchived}},
	}
	operations := corequeue.NewOperationsRuntime(manager)

	tasks, err := operations.ListFailedTasks(context.Background(), corequeue.TaskQuery{Queue: corequeue.DefaultQueueName})
	if err != nil {
		t.Fatalf("failed tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "archived-1" {
		t.Fatalf("tasks = %#v", tasks)
	}

	manager.failedAsArch = false
	manager.tasks = nil
	_, err = operations.ListFailedTasks(context.Background(), corequeue.TaskQuery{Queue: corequeue.DefaultQueueName})
	if errors.Is(err, corequeue.ErrCapabilityUnsupported) {
		t.Fatalf("unexpected unsupported error: %v", err)
	}
}
