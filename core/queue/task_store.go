package queue

import (
	"context"
	"fmt"
	"time"
)

const (
	TaskLogLevelInfo  = "info"
	TaskLogLevelWarn  = "warn"
	TaskLogLevelError = "error"
)

type TaskStore interface {
	RecordEnqueued(ctx context.Context, record TaskRecord) error
	EnsureRunning(ctx context.Context, record TaskRecord) (TaskRecord, error)
	StartRun(ctx context.Context, run TaskRunRecord) (TaskRunRecord, error)
	FinishRun(ctx context.Context, run TaskRunRecord) error
	AppendRunLog(ctx context.Context, log TaskRunLogRecord) error
	UpdateTaskStatus(ctx context.Context, update TaskStatusUpdate) error
}

type TaskSubmissionStore interface {
	RecordSubmitting(ctx context.Context, record TaskRecord) (TaskRecord, error)
	RecordDispatched(ctx context.Context, record TaskRecord) error
	RecordDispatchFailed(ctx context.Context, record TaskRecord) error
}

type TaskScheduleStore interface {
	RecordScheduled(ctx context.Context, record TaskRecord) (TaskRecord, error)
	ClaimScheduled(ctx context.Context, now time.Time, limit int, workerID string, driver string) ([]TaskRecord, error)
}

type TaskAutoRetryStore interface {
	ClaimAutoRetryTasks(ctx context.Context, now time.Time, limit int, workerID string, driver string) ([]TaskRecord, error)
	MarkAutoRetryFailed(ctx context.Context, record TaskRecord, err error) error
}

type TaskRecord struct {
	RecordID              int64
	Driver                string
	TaskID                string
	Queue                 string
	Type                  string
	Payload               []byte
	State                 TaskState
	MaxRetry              int
	Attempts              int
	TimeoutSeconds        int
	UniqueSeconds         int
	DelaySeconds          int
	AutoRetryEnabled      bool
	AutoRetryCount        int
	AutoRetryMax          int
	AutoRetryDelaySeconds int
	NextRetryAt           *time.Time
	LastRetryError        string
	ScheduledAt           *time.Time
	StartedAt             *time.Time
	FinishedAt            *time.Time
	LastError             string
	WorkerID              string
	TrackID               string
	RequestID             string
	TraceID               string
	SpanID                string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	Headers               map[string]string
}

type TaskRunRecord struct {
	RunDBID     int64
	RunID       string
	QueueTaskID int64
	TaskID      string
	Queue       string
	Type        string
	Attempt     int
	Status      TaskState
	WorkerID    string
	StartedAt   time.Time
	FinishedAt  time.Time
	DurationMS  int64
	Error       string
	TrackID     string
	RequestID   string
	TraceID     string
	SpanID      string
	Headers     map[string]string
}

type TaskRunLogRecord struct {
	QueueTaskID    int64
	QueueTaskRunID int64
	RunID          string
	TaskID         string
	Level          string
	Message        string
	Fields         map[string]any
	TrackID        string
	RequestID      string
	TraceID        string
	SpanID         string
	At             time.Time
	Headers        map[string]string
}

type TaskStatusUpdate struct {
	Queue     string
	TaskID    string
	Status    TaskState
	LastError string
	UpdatedAt time.Time
}

type TaskLogger interface {
	Info(message string, fields ...any)
	Warn(message string, fields ...any)
	Error(message string, fields ...any)
}

type taskLoggerKey struct{}

func WithTaskLogger(ctx context.Context, logger TaskLogger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, taskLoggerKey{}, logger)
}

func TaskLoggerFromContext(ctx context.Context) TaskLogger {
	if ctx != nil {
		if logger, ok := ctx.Value(taskLoggerKey{}).(TaskLogger); ok && logger != nil {
			return logger
		}
	}
	return NoopTaskLogger{}
}

type NoopTaskLogger struct{}

func (NoopTaskLogger) Info(string, ...any)  {}
func (NoopTaskLogger) Warn(string, ...any)  {}
func (NoopTaskLogger) Error(string, ...any) {}

type TaskStoreOptions struct {
	Driver   string
	WorkerID string
	RunID    func() string
	Now      func() time.Time
}

func TaskStoreMiddleware(store TaskStore, opts TaskStoreOptions) Middleware {
	if store == nil {
		return nil
	}
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, msg *Message) error {
			now := taskStoreNow(opts)
			task, run, startErr := startStoredTaskRun(ctx, store, opts, msg, now)
			if startErr != nil {
				return next(ctx, msg)
			}

			runCtx := ctx
			if run.RunID != "" {
				runCtx = WithTaskLogger(ctx, &taskRunLogger{
					ctx:   ctx,
					store: store,
					msg:   msg,
					run:   run,
				})
			}

			var err error
			defer func() {
				finishedAt := taskStoreNow(opts)
				if recovered := recover(); recovered != nil {
					err = fmt.Errorf("panic: %v", recovered)
					_ = finishStoredTaskRun(runCtx, store, task, run, msg, now, finishedAt, err)
					panic(recovered)
				}
				_ = finishStoredTaskRun(runCtx, store, task, run, msg, now, finishedAt, err)
			}()
			err = next(runCtx, msg)
			return err
		}
	}
}

func RecordTaskEnqueued(ctx context.Context, store TaskStore, record TaskRecord) error {
	if store == nil {
		return nil
	}
	if record.State == "" {
		record.State = StatePending
	}
	record = normalizeTaskRecord(ctx, record)
	return store.RecordEnqueued(ctx, record)
}

func RecordTaskSubmitting(ctx context.Context, store TaskStore, record TaskRecord) (TaskRecord, error) {
	submissionStore, ok := store.(TaskSubmissionStore)
	if !ok || submissionStore == nil {
		return record, nil
	}
	record = normalizeTaskRecord(ctx, record)
	record.State = StateSubmitting
	return submissionStore.RecordSubmitting(ctx, record)
}

func RecordTaskScheduled(ctx context.Context, store TaskStore, record TaskRecord) (TaskRecord, error) {
	scheduleStore, ok := store.(TaskScheduleStore)
	if !ok || scheduleStore == nil {
		return record, nil
	}
	record = normalizeTaskRecord(ctx, record)
	record.State = StateScheduled
	return scheduleStore.RecordScheduled(ctx, record)
}

func RecordTaskDispatched(ctx context.Context, store TaskStore, record TaskRecord) error {
	submissionStore, ok := store.(TaskSubmissionStore)
	if !ok || submissionStore == nil {
		return nil
	}
	record = normalizeTaskRecord(ctx, record)
	return submissionStore.RecordDispatched(ctx, record)
}

func RecordTaskDispatchFailed(ctx context.Context, store TaskStore, record TaskRecord) error {
	submissionStore, ok := store.(TaskSubmissionStore)
	if !ok || submissionStore == nil {
		return nil
	}
	record = normalizeTaskRecord(ctx, record)
	record.State = StateSubmitFailed
	return submissionStore.RecordDispatchFailed(ctx, record)
}

func UpdateStoredTaskStatus(ctx context.Context, store TaskStore, update TaskStatusUpdate) error {
	if store == nil || update.TaskID == "" || update.Status == "" {
		return nil
	}
	if update.UpdatedAt.IsZero() {
		update.UpdatedAt = time.Now()
	}
	return store.UpdateTaskStatus(ctx, update)
}

func normalizeTaskRecord(ctx context.Context, record TaskRecord) TaskRecord {
	if record.Headers == nil {
		record.Headers = CorrelationHeadersFromContext(ctx)
	}
	if record.TrackID == "" {
		record.TrackID = TrackIDFromHeaders(record.Headers)
	}
	if record.RequestID == "" {
		record.RequestID = RequestIDFromHeaders(record.Headers)
	}
	if record.TraceID == "" {
		record.TraceID = TraceIDFromHeaders(record.Headers)
	}
	if record.SpanID == "" {
		record.SpanID = SpanIDFromHeaders(record.Headers)
	}
	now := time.Now()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	return record
}

func startStoredTaskRun(ctx context.Context, store TaskStore, opts TaskStoreOptions, msg *Message, startedAt time.Time) (TaskRecord, TaskRunRecord, error) {
	if msg == nil {
		return TaskRecord{}, TaskRunRecord{}, nil
	}
	headers := msg.Headers
	if headers == nil {
		headers = CorrelationHeadersFromContext(ctx)
	}
	task, err := store.EnsureRunning(ctx, TaskRecord{
		Driver:    opts.Driver,
		TaskID:    msg.ID,
		Queue:     taskStoreFirstNonEmpty(msg.Queue, DefaultQueueName),
		Type:      msg.Type,
		Payload:   msg.Payload,
		State:     StateActive,
		MaxRetry:  msg.MaxRetry,
		Attempts:  msg.RetryCount + 1,
		StartedAt: &startedAt,
		WorkerID:  opts.WorkerID,
		TrackID:   TrackIDFromHeaders(headers),
		RequestID: RequestIDFromHeaders(headers),
		TraceID:   TraceIDFromHeaders(headers),
		SpanID:    SpanIDFromHeaders(headers),
		CreatedAt: startedAt,
		UpdatedAt: startedAt,
		Headers:   headers,
	})
	if err != nil {
		return TaskRecord{}, TaskRunRecord{}, err
	}
	runID := ""
	if opts.RunID != nil {
		runID = opts.RunID()
	}
	run, err := store.StartRun(ctx, TaskRunRecord{
		RunID:       runID,
		QueueTaskID: task.RecordID,
		TaskID:      msg.ID,
		Queue:       taskStoreFirstNonEmpty(msg.Queue, DefaultQueueName),
		Type:        msg.Type,
		Attempt:     msg.RetryCount + 1,
		Status:      StateActive,
		WorkerID:    opts.WorkerID,
		StartedAt:   startedAt,
		TrackID:     TrackIDFromHeaders(headers),
		RequestID:   RequestIDFromHeaders(headers),
		TraceID:     TraceIDFromHeaders(headers),
		SpanID:      SpanIDFromHeaders(headers),
		Headers:     headers,
	})
	if err != nil {
		return task, TaskRunRecord{}, err
	}
	_ = store.AppendRunLog(ctx, taskLogFromRun(run, msg, TaskLogLevelInfo, "queue task started", nil, startedAt))
	return task, run, nil
}

func finishStoredTaskRun(ctx context.Context, store TaskStore, task TaskRecord, run TaskRunRecord, msg *Message, startedAt time.Time, finishedAt time.Time, runErr error) error {
	if msg == nil || run.RunID == "" {
		return nil
	}
	status := StateSucceeded
	level := TaskLogLevelInfo
	message := "queue task completed"
	errText := ""
	if runErr != nil {
		status = StateFailed
		if IsRateLimitError(runErr) || msg.RetryCount < msg.MaxRetry {
			status = StateRetry
		}
		level = TaskLogLevelError
		message = "queue task failed"
		errText = runErr.Error()
	}
	run.Status = status
	run.FinishedAt = finishedAt
	run.DurationMS = finishedAt.Sub(startedAt).Milliseconds()
	run.Error = errText
	if err := store.FinishRun(ctx, run); err != nil {
		return err
	}
	fields := map[string]any{
		"attempt":     msg.RetryCount + 1,
		"max_retry":   msg.MaxRetry,
		"duration_ms": run.DurationMS,
	}
	if errText != "" {
		fields["error"] = errText
	}
	if err := store.AppendRunLog(ctx, taskLogFromRun(run, msg, level, message, fields, finishedAt)); err != nil {
		return err
	}
	return store.UpdateTaskStatus(ctx, TaskStatusUpdate{
		Queue:     msg.Queue,
		TaskID:    msg.ID,
		Status:    status,
		LastError: errText,
		UpdatedAt: finishedAt,
	})
}

func taskLogFromRun(run TaskRunRecord, msg *Message, level string, message string, fields map[string]any, at time.Time) TaskRunLogRecord {
	return TaskRunLogRecord{
		QueueTaskID:    run.QueueTaskID,
		QueueTaskRunID: run.RunDBID,
		RunID:          run.RunID,
		TaskID:         run.TaskID,
		Level:          level,
		Message:        message,
		Fields:         fields,
		TrackID:        run.TrackID,
		RequestID:      run.RequestID,
		TraceID:        run.TraceID,
		SpanID:         run.SpanID,
		At:             at,
		Headers:        msg.Headers,
	}
}

type taskRunLogger struct {
	ctx   context.Context
	store TaskStore
	msg   *Message
	run   TaskRunRecord
}

func (l *taskRunLogger) Info(message string, fields ...any) {
	l.write(TaskLogLevelInfo, message, fields...)
}

func (l *taskRunLogger) Warn(message string, fields ...any) {
	l.write(TaskLogLevelWarn, message, fields...)
}

func (l *taskRunLogger) Error(message string, fields ...any) {
	l.write(TaskLogLevelError, message, fields...)
}

func (l *taskRunLogger) write(level string, message string, fields ...any) {
	if l == nil || l.store == nil || l.msg == nil {
		return
	}
	_ = l.store.AppendRunLog(l.ctx, taskLogFromRun(l.run, l.msg, level, message, fieldsToMap(fields...), time.Now()))
}

func fieldsToMap(fields ...any) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	if len(fields) == 1 {
		if m, ok := fields[0].(map[string]any); ok {
			return m
		}
	}
	out := make(map[string]any, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if !ok || key == "" {
			continue
		}
		out[key] = fields[i+1]
	}
	return out
}

func taskStoreNow(opts TaskStoreOptions) time.Time {
	if opts.Now != nil {
		return opts.Now()
	}
	return time.Now()
}

func taskStoreFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
