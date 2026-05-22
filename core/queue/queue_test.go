package queue

import (
	"context"
	"errors"
	"testing"
	"time"
)

type payloadFixture struct {
	UserID int64  `json:"user_id"`
	Source string `json:"source"`
	Force  bool   `json:"force"`
}

type testQueueRunner struct {
	closed bool
}

func (r *testQueueRunner) Enqueue(context.Context, Task, ...Option) (*TaskInfo, error) {
	return &TaskInfo{ID: "test-task"}, nil
}

func (r *testQueueRunner) BatchEnqueue(context.Context, []Task, ...Option) ([]*TaskInfo, error) {
	return []*TaskInfo{{ID: "test-task"}}, nil
}

func (r *testQueueRunner) Close() error {
	r.closed = true
	return nil
}

func (r *testQueueRunner) Handle(string, HandlerFunc) {}

func (r *testQueueRunner) Use(...Middleware) {}

func (r *testQueueRunner) Run(context.Context) error { return nil }

func (r *testQueueRunner) Shutdown(context.Context) error { return nil }

func (r *testQueueRunner) Supports(cap Capability) bool {
	return r.Capabilities()[cap]
}

func (r *testQueueRunner) Capabilities() map[Capability]bool {
	return map[Capability]bool{CapEnqueue: true, CapConsume: true, CapInspector: true}
}

func (r *testQueueRunner) ListQueues(context.Context) ([]*QueueInfo, error) { return nil, nil }

func (r *testQueueRunner) GetQueue(context.Context, string) (*QueueInfo, error) { return nil, nil }

func (r *testQueueRunner) ListTasks(context.Context, TaskQuery) ([]*TaskInfo, error) {
	return nil, nil
}

func (r *testQueueRunner) GetTask(context.Context, string, string) (*TaskInfo, error) {
	return nil, nil
}

func (r *testQueueRunner) DeleteTask(context.Context, string, string) error { return nil }

func (r *testQueueRunner) RetryTask(context.Context, string, string) error { return nil }

func (r *testQueueRunner) ArchiveTask(context.Context, string, string) error { return nil }

func (r *testQueueRunner) CancelTask(context.Context, string, string) error { return nil }

func (r *testQueueRunner) PauseQueue(context.Context, string) error { return nil }

func (r *testQueueRunner) ResumeQueue(context.Context, string) error { return nil }

type testQueueClient struct {
	enqueued int
	closed   bool
}

func (c *testQueueClient) Enqueue(context.Context, Task, ...Option) (*TaskInfo, error) {
	c.enqueued++
	return &TaskInfo{ID: "client-task"}, nil
}

func (c *testQueueClient) BatchEnqueue(context.Context, []Task, ...Option) ([]*TaskInfo, error) {
	c.enqueued++
	return []*TaskInfo{{ID: "client-task"}}, nil
}

func (c *testQueueClient) Close() error {
	c.closed = true
	return nil
}

type testDriver struct {
	name   string
	runner QueueRunner
}

func (d testDriver) Name() string { return d.name }

func (d testDriver) Capabilities() map[Capability]bool { return d.runner.Capabilities() }

func (d testDriver) Supports(cap Capability) bool { return d.runner.Supports(cap) }

func (d testDriver) NewClient(Config) (Client, error) { return d.runner, nil }

func (d testDriver) NewWorker(Config, WorkerProfile) (Worker, error) { return d.runner, nil }

func (d testDriver) NewManager(Config) (Manager, error) { return d.runner, nil }

func (d testDriver) NewRunner(Config, ...RuntimeOption) (QueueRunner, error) {
	return d.runner, nil
}

type countingDriver struct {
	name      string
	client    Client
	manager   Manager
	runner    QueueRunner
	runtime   RuntimeOptions
	newClient int
	newRunner int
}

func (d *countingDriver) Name() string { return d.name }

func (d *countingDriver) Capabilities() map[Capability]bool {
	if d.runner != nil {
		return d.runner.Capabilities()
	}
	return map[Capability]bool{CapEnqueue: true, CapInspector: true}
}

func (d *countingDriver) Supports(cap Capability) bool { return d.Capabilities()[cap] }

func (d *countingDriver) NewClient(Config) (Client, error) {
	d.newClient++
	return d.client, nil
}

func (d *countingDriver) NewWorker(Config, WorkerProfile) (Worker, error) {
	return d.runner, nil
}

func (d *countingDriver) NewManager(Config) (Manager, error) {
	return d.manager, nil
}

func (d *countingDriver) NewRunner(_ Config, opts ...RuntimeOption) (QueueRunner, error) {
	d.newRunner++
	d.runtime = ApplyRuntimeOptions(opts)
	return d.runner, nil
}

type recordingQueueClient struct {
	tasks []Task
	opts  []EnqueueOptions
}

func (c *recordingQueueClient) Enqueue(_ context.Context, task Task, opts ...Option) (*TaskInfo, error) {
	c.tasks = append(c.tasks, task)
	c.opts = append(c.opts, ApplyOptions(opts))
	return &TaskInfo{ID: task.ID, Type: task.Type, Queue: ApplyOptions(opts).Queue}, nil
}

func (c *recordingQueueClient) BatchEnqueue(ctx context.Context, tasks []Task, opts ...Option) ([]*TaskInfo, error) {
	out := make([]*TaskInfo, 0, len(tasks))
	for _, task := range tasks {
		info, err := c.Enqueue(ctx, task, opts...)
		if err != nil {
			return out, err
		}
		out = append(out, info)
	}
	return out, nil
}

func (c *recordingQueueClient) Close() error {
	return nil
}

type recordingQueueWorker struct {
	handlers    map[string]HandlerFunc
	middlewares []Middleware
}

func (w *recordingQueueWorker) Handle(pattern string, handler HandlerFunc) {
	if w.handlers == nil {
		w.handlers = map[string]HandlerFunc{}
	}
	w.handlers[pattern] = handler
}

func (w *recordingQueueWorker) Use(middlewares ...Middleware) {
	w.middlewares = append(w.middlewares, middlewares...)
}

func (w *recordingQueueWorker) Run(context.Context) error {
	return nil
}

func (w *recordingQueueWorker) Shutdown(context.Context) error {
	return nil
}

func TestMarshalPayloadSupportsDocumentedPayloadTypes(t *testing.T) {
	tests := []struct {
		name    string
		payload any
		want    []byte
	}{
		{name: "nil", payload: nil, want: nil},
		{name: "bytes", payload: []byte(`{"raw":true}`), want: []byte(`{"raw":true}`)},
		{name: "string", payload: "plain-text", want: []byte("plain-text")},
		{name: "struct", payload: payloadFixture{UserID: 1001, Source: "admin", Force: true}, want: []byte(`{"user_id":1001,"source":"admin","force":true}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MarshalPayload(tt.payload)
			if err != nil {
				t.Fatalf("MarshalPayload() error = %v", err)
			}
			if string(got) != string(tt.want) {
				t.Fatalf("MarshalPayload() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodePayload(t *testing.T) {
	msg := &Message{Payload: []byte(`{"user_id":1001,"source":"admin","force":true}`)}

	got, err := DecodePayload[payloadFixture](msg)
	if err != nil {
		t.Fatalf("DecodePayload() error = %v", err)
	}
	if got != (payloadFixture{UserID: 1001, Source: "admin", Force: true}) {
		t.Fatalf("DecodePayload() = %+v", got)
	}

	if _, err := DecodePayload[payloadFixture](nil); err == nil {
		t.Fatal("DecodePayload(nil) error = nil, want error")
	}
}

func TestTypedPayloadHandlerDecodesPayloadAndStoresMessageContext(t *testing.T) {
	var got payloadFixture
	var gotMessage *Message

	wrapped, err := BuildHandlerChain(func(ctx context.Context, payload *payloadFixture) error {
		if payload == nil {
			t.Fatal("payload is nil")
		}
		got = *payload
		gotMessage, _ = MessageFromContext(ctx)
		return nil
	})
	if err != nil {
		t.Fatalf("BuildHandlerChain() error = %v", err)
	}
	msg := &Message{
		ID:      "task-1",
		Type:    "user:sync",
		Payload: []byte(`{"user_id":1001,"source":"admin","force":true}`),
	}
	if err := wrapped(context.Background(), msg); err != nil {
		t.Fatalf("wrapped() error = %v", err)
	}
	if got != (payloadFixture{UserID: 1001, Source: "admin", Force: true}) {
		t.Fatalf("payload = %+v", got)
	}
	if gotMessage != msg {
		t.Fatalf("message context = %#v, want %#v", gotMessage, msg)
	}
}

func TestRegistryRegistersTypedEventHandler(t *testing.T) {
	worker := &recordingQueueWorker{}
	registry := NewRegistry(worker)
	middlewareCalled := false
	registry.Use(func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, msg *Message) error {
			middlewareCalled = true
			return next(ctx, msg)
		}
	})

	called := false
	if err := registry.Register("user:sync", func(_ context.Context, payload *payloadFixture) error {
		called = payload != nil && payload.UserID == 1001
		return nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	handler := worker.handlers["user:sync"]
	if handler == nil {
		t.Fatal("registered handler missing")
	}
	if err := handler(context.Background(), &Message{Payload: []byte(`{"user_id":1001}`)}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if !called {
		t.Fatal("typed event handler was not called")
	}
	if !middlewareCalled {
		t.Fatal("registry middleware was not called")
	}
	if len(registry.Entries()) != 1 || registry.Entries()[0].Pattern != "user:sync" {
		t.Fatalf("registry entries = %+v", registry.Entries())
	}
	if _, ok := registry.Dispatcher().HandlerFor("user:sync"); !ok {
		t.Fatal("dispatcher handler missing")
	}
}

func TestNewRunnerUsesRegisteredDriver(t *testing.T) {
	runner := &testQueueRunner{}
	if err := RegisterDriver(testDriver{name: "test-driver-new-runner", runner: runner}); err != nil {
		t.Fatalf("RegisterDriver() error = %v", err)
	}

	got, err := NewRunner(Config{Driver: " test-driver-new-runner "})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	if got != runner {
		t.Fatalf("NewRunner() = %#v, want %#v", got, runner)
	}
}

func TestNewRunnerRejectsNilDriverRunner(t *testing.T) {
	driver := &countingDriver{name: "test-driver-nil-runner"}
	if err := RegisterDriver(driver); err != nil {
		t.Fatalf("RegisterDriver() error = %v", err)
	}

	if _, err := NewRunner(Config{Driver: driver.name}); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("NewRunner() error = %v, want ErrNotInitialized", err)
	}

	runner := New(Config{Driver: driver.name})
	_, err := runner.Enqueue(context.Background(), NewTask("example:task", nil))
	if !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("New().Enqueue() error = %v, want ErrNotInitialized", err)
	}
}

func TestQueuePushAndDelayUseContextRuntime(t *testing.T) {
	client := &recordingQueueClient{}
	ctx := ContextWithRuntime(context.Background(), NewRuntimeInstanceFromParts(RuntimeParts{Client: client}))

	if _, err := Push(ctx, "user:sync", payloadFixture{UserID: 1001}, Queue("critical")); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if _, err := Delay(ctx, "user:sync", payloadFixture{UserID: 1002}, time.Minute, Unique(5*time.Minute)); err != nil {
		t.Fatalf("Delay() error = %v", err)
	}
	if len(client.tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(client.tasks))
	}
	if client.tasks[0].Type != "user:sync" || client.opts[0].Queue != "critical" {
		t.Fatalf("Push task=%+v opts=%+v", client.tasks[0], client.opts[0])
	}
	if client.opts[1].ProcessIn != time.Minute || client.opts[1].UniqueTTL != 5*time.Minute {
		t.Fatalf("Delay opts=%+v", client.opts[1])
	}
}

func TestNewReturnsUnavailableRunnerForUnknownDriver(t *testing.T) {
	runner := New(Config{Driver: "missing-new-driver"})
	_, err := runner.Enqueue(context.Background(), NewTask("example:task", nil))
	if !errors.Is(err, ErrCapabilityUnsupported) {
		t.Fatalf("Enqueue() error = %v, want ErrCapabilityUnsupported", err)
	}
}

func TestConfigNormalizeAndFromConfig(t *testing.T) {
	if got := (Config{}).Normalize(); got.Concurrency != defaultConcurrency || got.Queues[DefaultQueueName] != 1 {
		t.Fatalf("normalize defaults = %+v", got)
	}

	cfg := FromConfig(&Config{
		Addr:           "127.0.0.1:6379",
		Password:       "secret",
		DB:             2,
		Concurrency:    3,
		Queues:         map[string]int{"critical": 10},
		StrictPriority: true,
	})
	if cfg.Addr != "127.0.0.1:6379" ||
		cfg.Password != "secret" ||
		cfg.DB != 2 ||
		cfg.Concurrency != 3 ||
		cfg.Queues["critical"] != 10 ||
		!cfg.StrictPriority {
		t.Fatalf("FromConfig() = %+v", cfg)
	}
}

func TestApplyOptionsMatchDocumentedOptions(t *testing.T) {
	deadline := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	processAt := deadline.Add(time.Hour)

	opts := applyOptions([]Option{
		Queue("critical"),
		MaxRetry(-1),
		Timeout(30 * time.Second),
		Deadline(deadline),
		ProcessAt(processAt),
		ProcessIn(5 * time.Minute),
		TaskID("task-1"),
		Unique(10 * time.Minute),
		Retention(time.Hour),
		Group("users"),
		WithPriority(7),
		WithRateLimitKey("tenant:1"),
		WithTrace(false),
	})

	if opts.Queue != "critical" ||
		opts.MaxRetry == nil ||
		*opts.MaxRetry != 0 ||
		opts.Timeout != 30*time.Second ||
		!opts.Deadline.Equal(deadline) ||
		!opts.ProcessAt.Equal(processAt) ||
		opts.ProcessIn != 5*time.Minute ||
		opts.TaskID != "task-1" ||
		opts.UniqueTTL != 10*time.Minute ||
		opts.Retention != time.Hour ||
		opts.Group != "users" ||
		opts.Priority != 7 ||
		opts.RateLimitKey != "tenant:1" ||
		opts.Trace {
		t.Fatalf("applyOptions() = %+v", opts)
	}
}

func TestRateLimitedError(t *testing.T) {
	inner := errors.New("remote service rate limited")
	err := RateLimited(2*time.Minute, inner)

	if !IsRateLimitError(err) {
		t.Fatal("IsRateLimitError() = false, want true")
	}
	if !errors.Is(err, inner) {
		t.Fatal("RateLimitError does not unwrap inner error")
	}
}

func TestContextChainPassesErrorBackToMiddleware(t *testing.T) {
	wantErr := errors.New("handler failed")
	seen := false

	wrapped := ContextChain(func(c *HandlerContext) error {
		err := c.Next()
		if errors.Is(err, wantErr) {
			seen = true
		}
		return err
	})(func(context.Context, *Message) error {
		return wantErr
	})

	err := wrapped(context.Background(), &Message{Type: "example:task"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("wrapped error = %v, want %v", err, wantErr)
	}
	if !seen {
		t.Fatal("middleware did not receive downstream error")
	}
}

func TestBuildHandlerChainAcceptsMiddlewareBeforeHandler(t *testing.T) {
	var order []string
	middleware := func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, msg *Message) error {
			order = append(order, "before")
			err := next(ctx, msg)
			order = append(order, "after")
			return err
		}
	}
	contextMiddleware := ContextHandler(func(c *HandlerContext) error {
		order = append(order, "context-before")
		err := c.Next()
		order = append(order, "context-after")
		return err
	})
	handler := func(context.Context, *Message) error {
		order = append(order, "handler")
		return nil
	}

	wrapped, err := BuildHandlerChain(middleware, contextMiddleware, handler)
	if err != nil {
		t.Fatalf("BuildHandlerChain() error = %v", err)
	}
	if err := wrapped(context.Background(), &Message{Type: "example:task"}); err != nil {
		t.Fatalf("wrapped() error = %v", err)
	}
	want := []string{"before", "context-before", "handler", "context-after", "after"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestBuildHandlerChainRejectsInvalidChain(t *testing.T) {
	if _, err := BuildHandlerChain(); err == nil {
		t.Fatal("BuildHandlerChain() error = nil, want error")
	}
	invalidFinal := func(context.Context, *Message) (string, int, time.Duration, bool, error) {
		return "", 0, 0, false, nil
	}
	if _, err := BuildHandlerChain(invalidFinal); err == nil {
		t.Fatal("BuildHandlerChain(invalid final) error = nil, want error")
	}
	if _, err := BuildHandlerChain(func(context.Context, *Message) error { return nil }, func(context.Context, *Message) error { return nil }); err == nil {
		t.Fatal("BuildHandlerChain(handler before final) error = nil, want error")
	}
}

func TestRuntimeOptionsDefaultRateLimitIsNotFailure(t *testing.T) {
	opts := ApplyRuntimeOptions(nil)

	if !opts.IsFailure(errors.New("ordinary failure")) {
		t.Fatal("ordinary error should count as failure")
	}
	if opts.IsFailure(RateLimited(time.Minute, errors.New("limited"))) {
		t.Fatal("rate limit error should not count as failure by default")
	}
}

func TestRuntimeKernelOwnsRateLimitState(t *testing.T) {
	runner := &testQueueRunner{}
	driver := &countingDriver{name: "test-runtime-kernel", runner: runner}
	if err := RegisterDriver(driver); err != nil {
		t.Fatalf("RegisterDriver() error = %v", err)
	}

	limiter := &testRateLimiter{}
	kernel, err := InitRuntimeKernel(context.Background(), Config{Driver: driver.name}, RuntimeKernelConfig{
		RateLimiter: limiter,
		RateLimitConfig: RateLimitConfig{
			Enabled:       true,
			DefaultLimit:  0,
			DefaultWindow: 0,
		},
	})
	if err != nil {
		t.Fatalf("InitRuntimeKernel() error = %v", err)
	}
	defer kernel.Close()

	if driver.newRunner != 1 || kernel.Runner() != runner {
		t.Fatalf("kernel runner newRunner=%d runner=%#v", driver.newRunner, kernel.Runner())
	}
	gotLimiter, gotRateLimit, ok := kernel.RateLimiter()
	if !ok || gotLimiter != limiter || gotRateLimit.DefaultLimit != 1 || gotRateLimit.DefaultWindow != time.Minute {
		t.Fatalf("RateLimiter() = %#v %+v %v", gotLimiter, gotRateLimit, ok)
	}
}

func TestRuntimeKernelClosesRunnerOnOutboxInitFailure(t *testing.T) {
	newRunner := &testQueueRunner{}
	driver := &countingDriver{name: "test-runtime-kernel-rollback", runner: newRunner}
	if err := RegisterDriver(driver); err != nil {
		t.Fatalf("RegisterDriver() error = %v", err)
	}
	wantErr := errors.New("migrate failed")
	_, err := InitRuntimeKernel(context.Background(), Config{
		Driver: driver.name,
		Outbox: OutboxConfig{Enabled: true},
	}, RuntimeKernelConfig{
		OutboxMigrate: func(context.Context) error {
			return wantErr
		},
		OutboxFactory: func(QueueRunner) (Outbox, error) {
			t.Fatal("outbox factory should not run after migrate failure")
			return nil, nil
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("InitRuntimeKernel() error = %v, want %v", err, wantErr)
	}
	if !newRunner.closed {
		t.Fatal("new runner was not closed after outbox failure")
	}
}

type testRateLimiter struct{}

func (l *testRateLimiter) Allow(context.Context, string, int, time.Duration) (bool, time.Duration, error) {
	return true, 0, nil
}
