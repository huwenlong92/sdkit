package crontab

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRegisterDispatchesDefaultTemplate(t *testing.T) {
	key := "runtime.register.template"
	called := false
	if err := Register(Template{
		Key:  key,
		Name: "Runtime Register Template",
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error {
			called = true
			return nil
		}),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	if err := Dispatch(context.Background(), &Entry{
		ID:          "manual.register.template",
		TemplateKey: key,
		Source:      SourceManual,
		Enabled:     true,
	}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("registered template handler was not called")
	}
}

func TestRuntimeDispatchUsesRegistryTemplate(t *testing.T) {
	registry := NewRegistry()
	called := false
	if err := registry.Register(Template{
		Key:  "runtime.cleanup",
		Name: "Runtime Cleanup",
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error {
			entry, ok := EntryFromContext(ctx)
			if !ok {
				t.Fatal("entry not found in context")
			}
			if entry.ID != "db.1" || entry.TemplateKey != "runtime.cleanup" {
				t.Fatalf("entry mismatch: %#v", entry)
			}
			called = true
			return nil
		}),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	runtime := NewRuntime(registry)
	err := runtime.Dispatch(context.Background(), &Entry{
		ID:          "db.1",
		TemplateKey: "runtime.cleanup",
		Source:      SourceDB,
		Enabled:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestRegistryDispatchesRunHandler(t *testing.T) {
	registry := NewRegistry()
	gotPayload := ""
	if err := registry.Register(Template{
		Key:  "runtime.dispatch",
		Name: "Runtime Dispatch",
		Handler: func(c *RunContext) RunResult {
			entry, ok := EntryFromContext(c.Context())
			if !ok {
				t.Fatal("entry not found in context")
			}
			gotPayload = c.Job.Payload
			if entry.Payload != c.Job.Payload {
				t.Fatalf("entry payload mismatch: %#v job=%#v", entry, c.Job)
			}
			return RunResult{Status: StatusSuccess}
		},
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	err := registry.Dispatch(context.Background(), &Entry{
		ID:          "db.1",
		TemplateKey: "runtime.dispatch",
		Source:      SourceDB,
		Enabled:     true,
		Payload:     `{"ok":true}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPayload != `{"ok":true}` {
		t.Fatalf("payload mismatch: got %q", gotPayload)
	}
}

func TestRunnerDispatchesRegisteredHandler(t *testing.T) {
	registry := NewRegistry()
	called := false
	if err := registry.Register(Template{
		Key:  "runtime.runner",
		Name: "Runtime Runner",
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error {
			entry, ok := EntryFromContext(ctx)
			if !ok {
				t.Fatal("entry not found in context")
			}
			if entry.TemplateKey != "runtime.runner" {
				t.Fatalf("template key mismatch: %#v", entry)
			}
			called = true
			return nil
		}),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	writer := &memoryLogWriter{}
	runner := NewRunner(RunnerOptions{Config: DefaultConfig(), Registry: registry, Logger: writer})
	runner.Run(context.Background(), Job{
		ID:      "db.1",
		Name:    "runtime.runner",
		Source:  SourceDB,
		Mode:    ModeLocal,
		Enabled: true,
	})

	if !called {
		t.Fatal("runtime handler was not called")
	}
	if got := writer.lastStatus(); got != StatusSuccess {
		t.Fatalf("status mismatch: got %s", got)
	}
}

func TestRuntimeGovernanceMetricsAndFailureCallback(t *testing.T) {
	resetRuntimeMetrics()
	defer resetRuntimeMetrics()
	defer UseFailureHandler()

	var reports []FailureReport
	UseFailureHandler(func(ctx context.Context, report FailureReport) {
		reports = append(reports, report)
	})

	registry := NewRegistry()
	if err := registry.Register(Template{
		Key:     "runtime.metrics",
		Name:    "Runtime Metrics",
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error { return nil }),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(Template{
		Key:  "runtime.metrics.failure",
		Name: "Runtime Metrics Failure",
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error {
			return errors.New("failed")
		}),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	if err := registry.Dispatch(context.Background(), &Entry{
		ID:          "db.1",
		TemplateKey: "runtime.metrics",
		Source:      SourceDB,
		Enabled:     true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Dispatch(context.Background(), &Entry{
		ID:          "db.2",
		TemplateKey: "runtime.metrics.failure",
		Source:      SourceDB,
		Enabled:     true,
	}); err == nil {
		t.Fatal("expected failure dispatch error")
	}

	metrics := RuntimeMetricsSnapshot()
	if metrics.CrontabExecuteTotal != 2 || metrics.CrontabExecuteSuccessTotal != 1 || metrics.CrontabExecuteFailedTotal != 1 {
		t.Fatalf("metrics mismatch: %#v", metrics)
	}
	if len(reports) != 1 {
		t.Fatalf("failure report count mismatch: %#v", reports)
	}
	report := reports[0]
	if report.EntryID != "db.2" || report.TemplateKey != "runtime.metrics.failure" || report.Error == nil {
		t.Fatalf("failure report mismatch: %#v", report)
	}
	if report.FinishedAt.Before(report.StartedAt) || report.Duration < 0 {
		t.Fatalf("failure report timing mismatch: %#v", report)
	}
}

func TestRuntimeMetricsTracksOverlapSkippedAndTimeout(t *testing.T) {
	resetRuntimeMetrics()
	defer resetRuntimeMetrics()

	lockRegistry := NewRegistry()
	if err := lockRegistry.Register(Template{
		Key:     "runtime.locked.metrics",
		Name:    "Runtime Locked Metrics",
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error { return nil }),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}
	lockRunner := NewRunner(RunnerOptions{
		Config:   DefaultConfig(),
		Registry: lockRegistry,
		Locker:   lockedLocker{},
		Logger:   &memoryLogWriter{},
	})
	lockRunner.Run(context.Background(), Job{
		ID:      "db.locked",
		Name:    "runtime.locked.metrics",
		Source:  SourceDB,
		Mode:    ModeLocal,
		Enabled: true,
	})

	timeoutRegistry := NewRegistry()
	if err := timeoutRegistry.Register(Template{
		Key:  "runtime.timeout.metrics",
		Name: "Runtime Timeout Metrics",
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error {
			<-ctx.Done()
			return ctx.Err()
		}),
		Enabled: true,
		AllowDB: true,
		Timeout: time.Nanosecond,
	}); err != nil {
		t.Fatal(err)
	}
	if err := timeoutRegistry.Dispatch(context.Background(), &Entry{
		ID:          "db.timeout",
		TemplateKey: "runtime.timeout.metrics",
		Source:      SourceDB,
		Enabled:     true,
	}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error mismatch: %v", err)
	}

	metrics := RuntimeMetricsSnapshot()
	if metrics.CrontabExecuteTotal != 2 || metrics.CrontabExecuteFailedTotal != 1 {
		t.Fatalf("execute metrics mismatch: %#v", metrics)
	}
	if metrics.CrontabOverlapSkippedTotal != 1 {
		t.Fatalf("overlap skipped metric mismatch: %#v", metrics)
	}
	if metrics.CrontabTimeoutTotal != 1 {
		t.Fatalf("timeout metric mismatch: %#v", metrics)
	}
}

func TestRuntimeGovernanceUsesEntryScopedLock(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Template{
		Key:     "runtime.lock",
		Name:    "Runtime Lock",
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error { return nil }),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	locker := &captureLocker{}
	runner := NewRunner(RunnerOptions{
		Config:   DefaultConfig(),
		Registry: registry,
		Locker:   locker,
		Logger:   &memoryLogWriter{},
	})
	runner.Run(context.Background(), Job{
		ID:      "db.42",
		Name:    "runtime.lock",
		Source:  SourceDB,
		Mode:    ModeLocal,
		Enabled: true,
	})

	if locker.key != "crontab:entry:db.42" {
		t.Fatalf("lock key mismatch: got %q", locker.key)
	}
}

type captureLocker struct {
	key string
}

func (l *captureLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (Lock, bool, error) {
	return l.TryLock(ctx, key, ttl)
}

func (l *captureLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (Lock, bool, error) {
	l.key = key
	return noopLock{}, true, nil
}
