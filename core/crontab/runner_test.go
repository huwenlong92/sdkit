package crontab

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/tracking"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type memoryLogWriter struct {
	mu   sync.Mutex
	logs []RunLog
}

func (w *memoryLogWriter) Write(ctx context.Context, log RunLog) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.logs = append(w.logs, log)
	return nil
}

func (w *memoryLogWriter) WriteBatch(ctx context.Context, logs []RunLog) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.logs = append(w.logs, logs...)
	return nil
}

func (w *memoryLogWriter) lastStatus() Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.logs) == 0 {
		return ""
	}
	return w.logs[len(w.logs)-1].Status
}

func (w *memoryLogWriter) lastLog() RunLog {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.logs) == 0 {
		return RunLog{}
	}
	return w.logs[len(w.logs)-1]
}

type memoryLogStore struct {
	events []LogEvent
}

func (s *memoryLogStore) Append(ctx context.Context, event LogEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *memoryLogStore) ListRunLogs(ctx context.Context, filter RunLogFilter) ([]LogEvent, error) {
	return s.events, nil
}

func TestRunnerLocalSuccess(t *testing.T) {
	registry := NewRegistry()
	called := false
	if err := registry.Register(Template{
		Name:    "local_job",
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error { called = true; return nil }),
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	writer := &memoryLogWriter{}
	runner := NewRunner(RunnerOptions{Config: DefaultConfig(), Registry: registry, Logger: writer})
	runner.Run(context.Background(), Job{Name: "local_job", Source: SourceBuiltin, Mode: ModeLocal, Enabled: true})

	if !called {
		t.Fatal("local handler was not called")
	}
	if got := writer.lastStatus(); got != StatusSuccess {
		t.Fatalf("status mismatch: got %s", got)
	}
}

func TestRunnerCreatesCronSpanAndPropagatesContext(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	oldProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(oldProvider)
	defer provider.Shutdown(context.Background())

	registry := NewRegistry()
	handlerTraceID := ""
	if err := registry.Register(Template{
		Name: "local_trace",
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error {
			spanCtx := oteltrace.SpanContextFromContext(ctx)
			if !spanCtx.IsValid() {
				t.Fatal("handler context missing cron span")
			}
			handlerTraceID = spanCtx.TraceID().String()
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
		ID:      "job-1",
		Name:    "local_trace",
		Source:  SourceDynamic,
		Mode:    ModeLocal,
		Spec:    "@every 1m",
		Enabled: true,
	})

	var cronSpan sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "crontab.execute" {
			cronSpan = span
			break
		}
	}
	if cronSpan == nil {
		t.Fatalf("missing crontab span: %+v", recorder.Ended())
	}
	if handlerTraceID == "" || handlerTraceID != cronSpan.SpanContext().TraceID().String() {
		t.Fatalf("handler trace id mismatch: handler=%s span=%s", handlerTraceID, cronSpan.SpanContext().TraceID())
	}
	assertRunSpanAttr(t, cronSpan, "template.name", "local_trace")
	assertRunSpanAttr(t, cronSpan, "entry.id", "job-1")
	assertRunSpanAttr(t, cronSpan, "cron", "@every 1m")
	assertRunSpanAttr(t, cronSpan, "crontab.status", string(StatusSuccess))
}

func TestRunnerPreservesTrackingID(t *testing.T) {
	registry := NewRegistry()
	gotTrackID := ""
	gotRunID := ""
	gotJobID := ""
	if err := registry.Register(Template{
		Name: "tracking_trace",
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error {
			gotTrackID = tracking.TrackID(ctx)
			gotRunID = logger.Field(ctx, logger.RunIDKey)
			gotJobID = logger.Field(ctx, logger.JobIDKey)
			JobLoggerFromContext(ctx).Info("tracked")
			return nil
		}),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	logStore := &memoryLogStore{}
	writer := &memoryLogWriter{}
	runner := NewRunner(RunnerOptions{Config: DefaultConfig(), Registry: registry, Logger: writer, LogStore: logStore})
	runner.Run(tracking.WithTrackID(context.Background(), "track-cron"), Job{
		ID:      "db.1",
		Name:    "tracking_trace",
		Source:  SourceDB,
		Mode:    ModeLocal,
		Enabled: true,
	})

	if gotTrackID != "track-cron" {
		t.Fatalf("track_id mismatch: got %s", gotTrackID)
	}
	if gotRunID == "" {
		t.Fatal("run_id should be present in handler context")
	}
	if gotJobID != "db.1" {
		t.Fatalf("job_id mismatch: got %s", gotJobID)
	}
	runLog := writer.lastLog()
	if runLog.RunID != gotRunID || runLog.JobID != "db.1" || runLog.TrackID != "track-cron" {
		t.Fatalf("run log missing correlation fields: %#v", runLog)
	}
	if len(logStore.events) != 1 || logStore.events[0].TrackID != "track-cron" {
		t.Fatalf("log event missing track_id: %#v", logStore.events)
	}
}

func TestRunnerTemplateLogDisabledSkipsRunAndJobLogs(t *testing.T) {
	registry := NewRegistry()
	called := false
	gotLogger := false
	if err := registry.Register(Template{
		Name:        "silent_template",
		LogDisabled: true,
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error {
			called = true
			JobLoggerFromContext(ctx).Info("should not persist")
			_, gotLogger = ctx.Value(jobLoggerKey{}).(JobLogger)
			return nil
		}),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	logStore := &memoryLogStore{}
	writer := &memoryLogWriter{}
	runtime := NewRuntimeState()
	runner := NewRunner(RunnerOptions{
		Config:   DefaultConfig(),
		Registry: registry,
		Logger:   writer,
		LogStore: logStore,
		Runtime:  runtime,
	})
	runner.Run(context.Background(), Job{
		ID:      "db.2",
		Name:    "silent_template",
		Source:  SourceDB,
		Mode:    ModeLocal,
		Enabled: true,
	})

	if !called {
		t.Fatal("handler was not called")
	}
	if !gotLogger {
		t.Fatal("job logger should be injected as noop")
	}
	if len(writer.logs) != 0 {
		t.Fatalf("run logs should be skipped: %#v", writer.logs)
	}
	if len(logStore.events) != 0 {
		t.Fatalf("job log events should be skipped: %#v", logStore.events)
	}
	if info, ok := runtime.Get("db.2"); !ok || info.Status != RuntimeSuccess {
		t.Fatalf("runtime state should still be updated: %#v ok=%v", info, ok)
	}
}

func TestRunnerGeneratesTrackingIDWhenMissing(t *testing.T) {
	registry := NewRegistry()
	gotTrackID := ""
	if err := registry.Register(Template{
		Name: "tracking_generate",
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error {
			gotTrackID = tracking.TrackID(ctx)
			return nil
		}),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(RunnerOptions{Config: DefaultConfig(), Registry: registry, Logger: &memoryLogWriter{}})
	runner.Run(context.Background(), Job{Name: "tracking_generate", Source: SourceDB, Mode: ModeLocal, Enabled: true})

	if gotTrackID == "" {
		t.Fatal("track_id should be generated")
	}
}

func assertRunSpanAttr(t *testing.T, span sdktrace.ReadOnlySpan, key string, want string) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key && attr.Value.AsString() == want {
			return
		}
	}
	t.Fatalf("missing span attr %s=%s: %+v", key, want, span.Attributes())
}

func TestRunnerTemplateMissing(t *testing.T) {
	writer := &memoryLogWriter{}
	runner := NewRunner(RunnerOptions{Config: DefaultConfig(), Registry: NewRegistry(), Logger: writer})
	runner.Run(context.Background(), Job{Name: "missing", Source: SourceDynamic, Mode: ModeLocal, Enabled: true})

	if got := writer.lastStatus(); got != StatusTemplateMissing {
		t.Fatalf("status mismatch: got %s", got)
	}
}

func TestRunnerFailureStatuses(t *testing.T) {
	tests := []struct {
		name string
		tpl  Template
		job  Job
		want Status
	}{
		{
			name: "template disabled",
			tpl:  Template{Name: "disabled", Handler: RunHandlerFromFunc(func(context.Context, Job) error { return nil }), Enabled: false, AllowDB: true},
			job:  Job{Name: "disabled", Source: SourceDynamic, Mode: ModeLocal, Enabled: true},
			want: StatusTemplateDisabled,
		},
		{
			name: "dynamic not allowed",
			tpl:  Template{Name: "not_dynamic", Handler: RunHandlerFromFunc(func(context.Context, Job) error { return nil }), Enabled: true, AllowDB: false},
			job:  Job{Name: "not_dynamic", Source: SourceDynamic, Mode: ModeLocal, Enabled: true},
			want: StatusSkipped,
		},
		{
			name: "job disabled",
			tpl:  Template{Name: "job_disabled", Handler: RunHandlerFromFunc(func(context.Context, Job) error { return nil }), Enabled: true, AllowDB: true},
			job:  Job{Name: "job_disabled", Source: SourceDynamic, Mode: ModeLocal, Enabled: false},
			want: StatusDisabled,
		},
		{
			name: "handler failed",
			tpl:  Template{Name: "failed", Handler: RunHandlerFromFunc(func(context.Context, Job) error { return errors.New("failed") }), Enabled: true, AllowDB: true},
			job:  Job{Name: "failed", Source: SourceDynamic, Mode: ModeLocal, Enabled: true},
			want: StatusFailed,
		},
		{
			name: "panic",
			tpl:  Template{Name: "panic", Handler: RunHandlerFromFunc(func(context.Context, Job) error { panic("boom") }), Enabled: true, AllowDB: true},
			job:  Job{Name: "panic", Source: SourceDynamic, Mode: ModeLocal, Enabled: true},
			want: StatusPanic,
		},
		{
			name: "handler missing",
			tpl:  Template{Name: "handler_missing", Enabled: true, AllowDB: true},
			job:  Job{Name: "handler_missing", Source: SourceDynamic, Mode: ModeLocal, Enabled: true},
			want: StatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			if err := registry.Register(tt.tpl); err != nil {
				t.Fatal(err)
			}
			writer := &memoryLogWriter{}
			runner := NewRunner(RunnerOptions{Config: DefaultConfig(), Registry: registry, Logger: writer})
			runner.Run(context.Background(), tt.job)
			if got := writer.lastStatus(); got != tt.want {
				t.Fatalf("status mismatch: got %s want %s", got, tt.want)
			}
		})
	}
}

func TestRunnerTimeout(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Template{
		Name:    "timeout",
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error { <-ctx.Done(); return ctx.Err() }),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	writer := &memoryLogWriter{}
	runner := NewRunner(RunnerOptions{Config: DefaultConfig(), Registry: registry, Logger: writer})
	runner.Run(context.Background(), Job{
		Name:    "timeout",
		Source:  SourceDynamic,
		Mode:    ModeLocal,
		Enabled: true,
		Timeout: time.Nanosecond,
	})

	if got := writer.lastStatus(); got != StatusTimeout {
		t.Fatalf("status mismatch: got %s", got)
	}
}

func TestRunnerLocked(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Template{
		Name:    "locked",
		Handler: RunHandlerFromFunc(func(context.Context, Job) error { return nil }),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.Lock.Enabled = true
	writer := &memoryLogWriter{}
	runner := NewRunner(RunnerOptions{
		Config:   cfg,
		Registry: registry,
		Locker:   lockedLocker{},
		Logger:   writer,
	})
	runner.Run(context.Background(), Job{Name: "locked", Source: SourceDynamic, Mode: ModeLocal, Enabled: true})

	if got := writer.lastStatus(); got != StatusLocked {
		t.Fatalf("status mismatch: got %s", got)
	}
}

func TestRunnerLockDisabledSkipsLocker(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Template{
		Name:    "lock_disabled",
		Handler: RunHandlerFromFunc(func(context.Context, Job) error { return nil }),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.Lock.Enabled = false
	writer := &memoryLogWriter{}
	runner := NewRunner(RunnerOptions{
		Config:   cfg,
		Registry: registry,
		Locker:   lockedLocker{},
		Logger:   writer,
	})
	runner.Run(context.Background(), Job{Name: "lock_disabled", Source: SourceDynamic, Mode: ModeLocal, Enabled: true})

	if got := writer.lastStatus(); got != StatusSuccess {
		t.Fatalf("status mismatch: got %s", got)
	}
}

func TestRunnerFailureWritesFinalLog(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Template{
		Name:    "failed",
		Handler: RunHandlerFromFunc(func(context.Context, Job) error { return errors.New("write failure log") }),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	writer := &memoryLogWriter{}
	runner := NewRunner(RunnerOptions{Config: DefaultConfig(), Registry: registry, Logger: writer})
	runner.Run(context.Background(), Job{Name: "failed", Source: SourceDynamic, Mode: ModeLocal, Enabled: true})

	log := writer.lastLog()
	if log.Status != StatusFailed {
		t.Fatalf("status mismatch: got %s", log.Status)
	}
	if log.Error != "write failure log" {
		t.Fatalf("error log mismatch: got %q", log.Error)
	}
	if log.FinishedAt == 0 || log.DurationMs < 0 {
		t.Fatalf("final log was not completed: %#v", log)
	}
}

func TestRunnerPanicWritesStack(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Template{
		Name:    "panic_log",
		Handler: RunHandlerFromFunc(func(context.Context, Job) error { panic("write panic log") }),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	writer := &memoryLogWriter{}
	runner := NewRunner(RunnerOptions{Config: DefaultConfig(), Registry: registry, Logger: writer})
	runner.Run(context.Background(), Job{Name: "panic_log", Source: SourceDynamic, Mode: ModeLocal, Enabled: true})

	log := writer.lastLog()
	if log.Status != StatusPanic {
		t.Fatalf("status mismatch: got %s", log.Status)
	}
	if log.Error == "" || log.PanicStack == "" {
		t.Fatalf("panic log missing error or stack: %#v", log)
	}
}

type lockedLocker struct{}

func (lockedLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (Lock, bool, error) {
	return nil, false, nil
}

func (lockedLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (Lock, bool, error) {
	return nil, false, nil
}
