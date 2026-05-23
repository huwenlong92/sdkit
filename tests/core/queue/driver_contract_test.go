package queue_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"
)

type externalDriver struct {
	client queue.Client
	runner queue.QueueRunner
}

func (d externalDriver) Name() string {
	return "external-contract-driver"
}

func (d externalDriver) Capabilities() map[queue.Capability]bool {
	return queue.CloneCapabilities(map[queue.Capability]bool{
		queue.CapEnqueue:   true,
		queue.CapConsume:   true,
		queue.CapInspector: true,
	})
}

func (d externalDriver) Supports(cap queue.Capability) bool {
	return d.Capabilities()[cap]
}

func (d externalDriver) NewClient(queue.Config) (queue.Client, error) {
	return d.client, nil
}

func (d externalDriver) NewWorker(queue.Config, queue.WorkerProfile) (queue.Worker, error) {
	return d.runner, nil
}

func (d externalDriver) NewManager(queue.Config) (queue.Manager, error) {
	return d.runner, nil
}

func (d externalDriver) NewRunner(queue.Config, ...queue.RuntimeOption) (queue.QueueRunner, error) {
	return d.runner, nil
}

type externalClient struct{}

func (externalClient) Enqueue(context.Context, queue.Task, ...queue.Option) (*queue.TaskInfo, error) {
	return &queue.TaskInfo{ID: "external-task"}, nil
}

func (externalClient) BatchEnqueue(context.Context, []queue.Task, ...queue.Option) ([]*queue.TaskInfo, error) {
	return []*queue.TaskInfo{{ID: "external-task"}}, nil
}

func (externalClient) Close() error {
	return nil
}

type externalRunner struct {
	externalClient
}

func (externalRunner) Handle(string, queue.HandlerFunc) {}

func (externalRunner) Use(...queue.Middleware) {}

func (externalRunner) Run(context.Context) error {
	return nil
}

func (externalRunner) Shutdown(context.Context) error {
	return nil
}

func (r externalRunner) Supports(cap queue.Capability) bool {
	return r.Capabilities()[cap]
}

func (externalRunner) Capabilities() map[queue.Capability]bool {
	return map[queue.Capability]bool{queue.CapEnqueue: true}
}

func (externalRunner) ListQueues(context.Context) ([]*queue.QueueInfo, error) {
	return []*queue.QueueInfo{{Name: queue.DefaultQueueName, State: queue.QueueRunning}}, nil
}

func (externalRunner) GetQueue(context.Context, string) (*queue.QueueInfo, error) {
	return &queue.QueueInfo{Name: queue.DefaultQueueName, State: queue.QueueRunning}, nil
}

func (externalRunner) ListTasks(context.Context, queue.TaskQuery) ([]*queue.TaskInfo, error) {
	return []*queue.TaskInfo{{ID: "external-task", State: queue.StatePending}}, nil
}

func (externalRunner) GetTask(context.Context, string, string) (*queue.TaskInfo, error) {
	return &queue.TaskInfo{ID: "external-task", State: queue.StatePending}, nil
}

func (externalRunner) DeleteTask(context.Context, string, string) error {
	return nil
}

func (externalRunner) RetryTask(context.Context, string, string) error {
	return nil
}

func (externalRunner) ArchiveTask(context.Context, string, string) error {
	return nil
}

func (externalRunner) CancelTask(context.Context, string, string) error {
	return nil
}

func (externalRunner) PauseQueue(context.Context, string) error {
	return nil
}

func (externalRunner) ResumeQueue(context.Context, string) error {
	return nil
}

func TestExternalDriverCanUsePublicQueueStandard(t *testing.T) {
	driver := externalDriver{
		client: externalClient{},
		runner: externalRunner{},
	}
	if err := queue.RegisterDriver(driver); err != nil {
		t.Fatalf("RegisterDriver() error = %v", err)
	}

	client, err := queue.NewClient(queue.Config{Driver: driver.Name()})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	info, err := client.Enqueue(context.Background(), queue.NewTask("external:task", map[string]any{"ok": true}))
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if info.ID != "external-task" {
		t.Fatalf("task id = %q", info.ID)
	}

	opts := queue.ApplyOptions([]queue.Option{
		queue.Queue("critical"),
		queue.TaskID("task-1"),
		queue.Timeout(time.Second),
	})
	if opts.Queue != "critical" || opts.TaskID != "task-1" || opts.Timeout != time.Second {
		t.Fatalf("ApplyOptions() = %+v", opts)
	}

	runtime := queue.ApplyRuntimeOptions([]queue.RuntimeOption{
		queue.WithIsFailure(func(err error) bool {
			return !errors.Is(err, context.Canceled)
		}),
	})
	if runtime.IsFailure(context.Canceled) {
		t.Fatal("custom IsFailure was not applied")
	}
	if !queue.DefaultRuntimeOptions().IsFailure(errors.New("ordinary failure")) {
		t.Fatal("DefaultRuntimeOptions should treat ordinary error as failure")
	}
}
