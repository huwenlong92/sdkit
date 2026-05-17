package asynq

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/logger"
	corequeue "github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"

	hibasynq "github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"
)

func TestAsynqOptionsMatchDocumentedOptions(t *testing.T) {
	deadline := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	processAt := deadline.Add(time.Hour)
	opts := corequeue.ApplyOptions([]corequeue.Option{
		corequeue.Queue("critical"),
		corequeue.MaxRetry(-1),
		corequeue.Timeout(30 * time.Second),
		corequeue.Deadline(deadline),
		corequeue.ProcessAt(processAt),
		corequeue.ProcessIn(5 * time.Minute),
		corequeue.TaskID("task-1"),
		corequeue.Unique(10 * time.Minute),
		corequeue.Retention(time.Hour),
		corequeue.Group("users"),
	})

	got := map[hibasynq.OptionType]interface{}{}
	for _, opt := range asynqOptions(opts) {
		got[opt.Type()] = opt.Value()
	}
	want := map[hibasynq.OptionType]interface{}{
		hibasynq.QueueOpt:     "critical",
		hibasynq.MaxRetryOpt:  0,
		hibasynq.TimeoutOpt:   30 * time.Second,
		hibasynq.DeadlineOpt:  deadline,
		hibasynq.ProcessAtOpt: processAt,
		hibasynq.ProcessInOpt: 5 * time.Minute,
		hibasynq.TaskIDOpt:    "task-1",
		hibasynq.UniqueOpt:    10 * time.Minute,
		hibasynq.RetentionOpt: time.Hour,
		hibasynq.GroupOpt:     "users",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("asynqOptions() = %#v, want %#v", got, want)
	}
}

func TestAsynqRetryDelay(t *testing.T) {
	err := corequeue.RateLimited(2*time.Minute, errors.New("limited"))
	delay := asynqRetryDelay()(1, err, hibasynq.NewTask("user:sync", []byte(`{}`)))
	if delay != 2*time.Minute {
		t.Fatalf("rate limit retry delay = %s, want 2m", delay)
	}

	runtimeErrDelay := asynqRetryDelay()(1, corequeue.RetryableAfter(5*time.Second, errors.New("retryable")), hibasynq.NewTask("user:sync", nil))
	if runtimeErrDelay != 5*time.Second {
		t.Fatalf("runtime retry delay = %s, want 5s", runtimeErrDelay)
	}
}

func TestMapAsynqStateAndTaskInfo(t *testing.T) {
	if got := mapAsynqState(hibasynq.TaskStateCompleted); got != corequeue.StateSucceeded {
		t.Fatalf("completed state = %s", got)
	}
	got := fromAsynqTaskInfo(&hibasynq.TaskInfo{
		ID:       "task-1",
		Queue:    "critical",
		Type:     "user:sync",
		State:    hibasynq.TaskStatePending,
		MaxRetry: 3,
		Retried:  1,
	})
	if got.ID != "task-1" ||
		got.Queue != "critical" ||
		got.Type != "user:sync" ||
		got.State != corequeue.StatePending ||
		got.MaxRetry != 3 ||
		got.Retried != 1 {
		t.Fatalf("fromAsynqTaskInfo() = %+v", got)
	}
	if fromAsynqTaskInfo(nil) != nil {
		t.Fatal("fromAsynqTaskInfo(nil) != nil")
	}
}

func TestHandleAsynqTaskCreatesWorkerSpanWithCorrelation(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	oldProvider := otel.GetTracerProvider()
	oldPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(tracing.NewPropagator())
	defer otel.SetTracerProvider(oldProvider)
	defer otel.SetTextMapPropagator(oldPropagator)
	defer provider.Shutdown(context.Background())

	ctx := tracking.WithTrackID(context.Background(), "track-worker")
	ctx = requestid.WithRequestID(ctx, "request-worker")
	ctx, parent := tracing.StartSpan(ctx, "producer")
	parentID := parent.SpanContext().SpanID()
	task := hibasynq.NewTaskWithHeaders("user:sync", []byte(`{"user_id":1}`), taskHeadersFromContext(ctx))

	err := handleAsynqTask(context.Background(), task, func(ctx context.Context, msg *corequeue.Message) error {
		if tracking.TrackID(ctx) != "track-worker" {
			t.Fatalf("track_id: want track-worker, got %s", tracking.TrackID(ctx))
		}
		if got := requestid.FromContext(ctx); got != "request-worker" {
			t.Fatalf("request_id: want request-worker, got %s", got)
		}
		if msg.Type != "user:sync" || string(msg.Payload) != `{"user_id":1}` {
			t.Fatalf("message = %+v payload=%s", msg, msg.Payload)
		}
		assertContextField(t, logger.ContextFields(ctx), logger.TypeKey, msg.Type)
		return nil
	})
	parent.End()
	if err != nil {
		t.Fatalf("handleAsynqTask() error = %v", err)
	}

	var workerSpan sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "consumer::user:sync" {
			workerSpan = span
			break
		}
	}
	if workerSpan == nil {
		t.Fatalf("missing worker span: %+v", recorder.Ended())
	}
	if workerSpan.Parent().SpanID() != parentID {
		t.Fatalf("worker parent: want %s, got %s", parentID, workerSpan.Parent().SpanID())
	}
	assertSpanAttrMissing(t, workerSpan, "sd.track_id")
	assertSpanAttrMissing(t, workerSpan, "sd.request_id")
	assertSpanAttr(t, workerSpan, "track_id", "track-worker")
	assertSpanAttr(t, workerSpan, "request_id", "request-worker")
	assertSpanAttr(t, workerSpan, "messaging.message.type", "user:sync")
	assertSpanAttrNotEmpty(t, workerSpan, "trace_id")
	assertSpanAttrMissing(t, workerSpan, "task_type")
	assertSpanAttrMissing(t, workerSpan, "task_id")
	assertSpanAttrMissing(t, workerSpan, "queue")
	assertSpanAttrNotEmpty(t, workerSpan, "traceparent")
}

func TestStartEnqueueSpanAddsQueueAttributes(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	oldProvider := otel.GetTracerProvider()
	oldPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(tracing.NewPropagator())
	defer otel.SetTracerProvider(oldProvider)
	defer otel.SetTextMapPropagator(oldPropagator)
	defer provider.Shutdown(context.Background())

	ctx := tracking.WithTrackID(context.Background(), "track-enqueue")
	ctx = requestid.WithRequestID(ctx, "request-enqueue")
	ctx, parent := tracing.StartSpan(ctx, "http.request")
	_, span := startEnqueueSpan(ctx, "user:sync", corequeue.EnqueueOptions{Queue: "default", TaskID: "task-1"})
	span.End()
	parent.End()

	var enqueueSpan sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "producer::user:sync" {
			enqueueSpan = span
			break
		}
	}
	if enqueueSpan == nil {
		t.Fatalf("missing enqueue span: %+v", recorder.Ended())
	}
	assertSpanAttr(t, enqueueSpan, "track_id", "track-enqueue")
	assertSpanAttr(t, enqueueSpan, "request_id", "request-enqueue")
	assertSpanAttrNotEmpty(t, enqueueSpan, "trace_id")
	assertSpanAttrMissing(t, enqueueSpan, "sd.track_id")
	assertSpanAttrMissing(t, enqueueSpan, "sd.request_id")
	assertSpanAttrMissing(t, enqueueSpan, "task_id")
	assertSpanAttrMissing(t, enqueueSpan, "task_type")
	assertSpanAttrMissing(t, enqueueSpan, "queue")
	assertSpanAttrNotEmpty(t, enqueueSpan, "traceparent")
}

func TestTaskMetaFromHeadersParsesCorrelationHeaders(t *testing.T) {
	meta := taskMetaFromHeaders(map[string]string{
		"x-track-id":   "track-queue",
		"X-Request-ID": "request-queue",
		"traceparent":  "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01",
	})

	if meta.TrackID != "track-queue" {
		t.Fatalf("track_id = %q, want track-queue", meta.TrackID)
	}
	if meta.RequestID != "request-queue" {
		t.Fatalf("request_id = %q, want request-queue", meta.RequestID)
	}
	if meta.TraceID != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("trace_id = %q", meta.TraceID)
	}
	if meta.SpanID != "bbbbbbbbbbbbbbbb" {
		t.Fatalf("span_id = %q", meta.SpanID)
	}
}

func TestContextWithMessageFieldsAddsQueueRuntimeFields(t *testing.T) {
	ctx := contextWithMessageFields(context.Background(), &corequeue.Message{
		ID:    "task-queue",
		Queue: "critical",
		Type:  "user:sync",
	})

	fields := logger.ContextFields(ctx)
	assertContextField(t, fields, logger.TaskIDKey, "task-queue")
	assertContextField(t, fields, logger.QueueKey, "critical")
	assertContextField(t, fields, logger.TypeKey, "user:sync")
}

func TestContextFromTaskHeadersAddsCorrelationFields(t *testing.T) {
	ctx := contextFromTaskHeaders(context.Background(), map[string]string{
		"X-Track-ID":   "track-queue",
		"X-Request-ID": "request-queue",
		"traceparent":  "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01",
	})

	fields := logger.ContextFields(ctx)
	assertContextField(t, fields, logger.TrackIDKey, "track-queue")
	assertContextField(t, fields, logger.RequestIDKey, "request-queue")
	assertContextField(t, fields, logger.TraceIDKey, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	assertContextField(t, fields, logger.SpanIDKey, "bbbbbbbbbbbbbbbb")
}

func assertSpanAttr(t *testing.T, span sdktrace.ReadOnlySpan, key string, want string) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key && attr.Value.AsString() == want {
			return
		}
	}
	t.Fatalf("missing span attr %s=%s: %+v", key, want, span.Attributes())
}

func assertContextField(t *testing.T, fields []zap.Field, key string, want string) {
	t.Helper()
	for _, field := range fields {
		if field.Key == key && field.String == want {
			return
		}
	}
	t.Fatalf("missing context field %s=%s: %+v", key, want, fields)
}

func assertSpanAttrNotEmpty(t *testing.T, span sdktrace.ReadOnlySpan, key string) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key && attr.Value.AsString() != "" {
			return
		}
	}
	t.Fatalf("missing non-empty span attr %s: %+v", key, span.Attributes())
}

func assertSpanAttrMissing(t *testing.T, span sdktrace.ReadOnlySpan, key string) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			t.Fatalf("unexpected span attr %s: %+v", key, span.Attributes())
		}
	}
}
