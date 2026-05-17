package operations

import (
	"context"
	"testing"

	corequeue "github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/runtime"
)

type testClient struct {
	closed bool
}

func (c *testClient) Enqueue(context.Context, corequeue.Task, ...corequeue.Option) (*corequeue.TaskInfo, error) {
	return &corequeue.TaskInfo{ID: "test-task"}, nil
}

func (c *testClient) BatchEnqueue(context.Context, []corequeue.Task, ...corequeue.Option) ([]*corequeue.TaskInfo, error) {
	return []*corequeue.TaskInfo{{ID: "test-task"}}, nil
}

func (c *testClient) Close() error {
	c.closed = true
	return nil
}

type testManager struct {
	closed bool
}

func (m *testManager) Supports(corequeue.Capability) bool { return false }
func (m *testManager) Capabilities() map[corequeue.Capability]bool {
	return map[corequeue.Capability]bool{}
}
func (m *testManager) ListQueues(context.Context) ([]*corequeue.QueueInfo, error) { return nil, nil }
func (m *testManager) GetQueue(context.Context, string) (*corequeue.QueueInfo, error) {
	return nil, corequeue.ErrNotInitialized
}
func (m *testManager) ListTasks(context.Context, corequeue.TaskQuery) ([]*corequeue.TaskInfo, error) {
	return nil, nil
}
func (m *testManager) GetTask(context.Context, string, string) (*corequeue.TaskInfo, error) {
	return nil, corequeue.ErrNotInitialized
}
func (m *testManager) DeleteTask(context.Context, string, string) error { return nil }
func (m *testManager) RetryTask(context.Context, string, string) error  { return nil }
func (m *testManager) ArchiveTask(context.Context, string, string) error {
	return nil
}
func (m *testManager) CancelTask(context.Context, string, string) error { return nil }
func (m *testManager) PauseQueue(context.Context, string) error         { return nil }
func (m *testManager) ResumeQueue(context.Context, string) error        { return nil }
func (m *testManager) Close() error {
	m.closed = true
	return nil
}

func TestUseRegistersQueueOperationsRuntime(t *testing.T) {
	client := &testClient{}
	manager := &testManager{}
	driver := testDriver{name: "operations-facade-test", client: client, manager: manager}
	corequeue.RegisterDriver(driver)

	app := runtime.New()
	if err := app.RegisterCapabilities(Use(
		WithName("admin.queue.operations"),
		WithConfig(NewConfig("admin", "admin", corequeue.Config{Driver: driver.name})),
	)); err != nil {
		t.Fatalf("RegisterCapabilities() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	value, ok := app.Container().Get(runtime.Key("admin.queue.operations"))
	if !ok {
		t.Fatal("admin.queue.operations was not bound")
	}
	queueRuntime, ok := value.(*corequeue.RuntimeInstance)
	if !ok {
		t.Fatalf("admin.queue.operations = %T, want *queue.RuntimeInstance", value)
	}
	if queueRuntime.Client() != client {
		t.Fatalf("runtime client = %v, want %v", queueRuntime.Client(), client)
	}
	if queueRuntime.Manager() != manager {
		t.Fatalf("runtime manager = %v, want %v", queueRuntime.Manager(), manager)
	}
	if got := queueRuntime.Metadata().Name; got != "admin" {
		t.Fatalf("metadata name = %q, want admin", got)
	}
	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !client.closed || !manager.closed {
		t.Fatalf("closed client=%v manager=%v, want true true", client.closed, manager.closed)
	}
}

func TestFromServiceContextReadsLocalOperationsRuntime(t *testing.T) {
	queueRuntime := corequeue.NewRuntimeInstanceFromParts(corequeue.RuntimeParts{
		Client:  &testClient{},
		Manager: &testManager{},
	})
	ctx := testServiceContext{values: map[string]any{
		"admin.queue.operations": queueRuntime,
	}}

	got, ok := FromServiceContext(ctx)
	if !ok || got != queueRuntime {
		t.Fatalf("FromServiceContext() = %v, %v; want runtime, true", got, ok)
	}
	ops, ok := OperationsFromServiceContext(ctx)
	if !ok || ops != queueRuntime.Operations() {
		t.Fatalf("OperationsFromServiceContext() = %v, %v; want operations, true", ops, ok)
	}
}

type testDriver struct {
	name    string
	client  corequeue.Client
	manager corequeue.Manager
}

func (d testDriver) Name() string { return d.name }
func (d testDriver) Capabilities() map[corequeue.Capability]bool {
	return map[corequeue.Capability]bool{}
}
func (d testDriver) Supports(corequeue.Capability) bool {
	return false
}
func (d testDriver) NewClient(corequeue.Config) (corequeue.Client, error) {
	return d.client, nil
}
func (d testDriver) NewWorker(corequeue.Config, corequeue.WorkerProfile) (corequeue.Worker, error) {
	return nil, corequeue.ErrNotInitialized
}
func (d testDriver) NewManager(corequeue.Config) (corequeue.Manager, error) {
	return d.manager, nil
}
func (d testDriver) NewRunner(corequeue.Config, ...corequeue.RuntimeOption) (corequeue.QueueRunner, error) {
	return nil, corequeue.ErrNotInitialized
}

type testServiceContext struct {
	values map[string]any
}

func (c testServiceContext) CapabilityLocalFirst(name string) (any, bool) {
	if value, ok := c.values[name]; ok {
		return value, true
	}
	value, ok := c.values["admin."+name]
	return value, ok
}
