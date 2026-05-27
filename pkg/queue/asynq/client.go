package asynq

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/tracing"

	hibasynq "github.com/hibiken/asynq"
)

type Queue struct {
	cfg       queue.Config
	runtime   queue.RuntimeOptions
	client    *hibasynq.Client
	server    *hibasynq.Server
	mux       *hibasynq.ServeMux
	inspector *hibasynq.Inspector
	mws       []queue.Middleware
}

func New(cfg queue.Config, opts ...queue.RuntimeOption) *Queue {
	cfg = cfg.Normalize()
	runtime := queue.ApplyRuntimeOptions(opts)
	redis := hibasynq.RedisClientOpt{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	return &Queue{
		cfg:       cfg,
		runtime:   runtime,
		client:    hibasynq.NewClient(redis),
		inspector: hibasynq.NewInspector(redis),
		server: hibasynq.NewServer(redis, hibasynq.Config{
			Concurrency:    cfg.Concurrency,
			Queues:         cfg.Queues,
			StrictPriority: cfg.StrictPriority,
			Logger:         logger.Asynq("asynq"),
			RetryDelayFunc: asynqRetryDelay(),
			IsFailure:      runtime.IsFailure,
		}),
		mux: hibasynq.NewServeMux(),
	}
}

func (q *Queue) Supports(cap queue.Capability) bool {
	return capabilities()[cap]
}

func (q *Queue) Capabilities() map[queue.Capability]bool {
	return queue.CloneCapabilities(capabilities())
}

func (q *Queue) Enqueue(ctx context.Context, task queue.Task, opts ...queue.Option) (*queue.TaskInfo, error) {
	if q == nil || q.client == nil {
		return nil, queue.ErrNotInitialized
	}
	if task.Type == "" {
		return nil, fmt.Errorf("queue: task type is required")
	}
	payload, err := queue.MarshalPayload(task.Payload)
	if err != nil {
		return nil, err
	}
	applied := queue.ApplyOptions(opts)
	if task.Queue != "" {
		applied.Queue = task.Queue
	}
	if task.ID != "" {
		applied.TaskID = task.ID
	}
	if err := validateOptions(applied); err != nil {
		return nil, err
	}
	ctx, span := startEnqueueSpan(ctx, task.Type, applied)
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(tracing.StatusError, err.Error())
		}
		span.End()
	}()

	headers := taskHeadersFromContext(ctx)
	for k, v := range task.Headers {
		if headers == nil {
			headers = map[string]string{}
		}
		headers[k] = v
	}
	asynqTask := hibasynq.NewTask(task.Type, payload)
	if len(headers) > 0 {
		asynqTask = hibasynq.NewTaskWithHeaders(task.Type, payload, headers)
	}
	info, err := q.client.EnqueueContext(ctx, asynqTask, asynqOptions(applied)...)
	if err != nil {
		return nil, mapEnqueueError(err)
	}
	out := fromAsynqTaskInfo(info)
	if out != nil {
		span.SetAttributes(
			tracing.String("messaging.message.id", out.ID),
			tracing.String("messaging.destination.name", out.Queue),
		)
	}
	return out, nil
}

func (q *Queue) BatchEnqueue(ctx context.Context, tasks []queue.Task, opts ...queue.Option) ([]*queue.TaskInfo, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	out := make([]*queue.TaskInfo, 0, len(tasks))
	for _, task := range tasks {
		info, err := q.Enqueue(ctx, task, opts...)
		if err != nil {
			return out, err
		}
		out = append(out, info)
	}
	return out, nil
}

func (q *Queue) Handle(pattern string, handler queue.HandlerFunc) {
	if q == nil || q.mux == nil {
		return
	}
	wrapped := handler
	for i := len(q.mws) - 1; i >= 0; i-- {
		wrapped = q.mws[i](wrapped)
	}
	q.mux.HandleFunc(pattern, func(ctx context.Context, task *hibasynq.Task) error {
		return handleAsynqTask(ctx, task, wrapped)
	})
}

func (q *Queue) Use(middlewares ...queue.Middleware) {
	if q == nil {
		return
	}
	q.mws = append(q.mws, middlewares...)
}

func handleAsynqTask(ctx context.Context, task *hibasynq.Task, handler queue.HandlerFunc) (err error) {
	ctx = contextFromTaskHeaders(ctx, task.Headers())

	id, _ := hibasynq.GetTaskID(ctx)
	queueName, _ := hibasynq.GetQueueName(ctx)
	retried, _ := hibasynq.GetRetryCount(ctx)
	maxRetry, _ := hibasynq.GetMaxRetry(ctx)
	msg := &queue.Message{
		ID:         id,
		Type:       task.Type(),
		Payload:    task.Payload(),
		Queue:      queueName,
		RetryCount: retried,
		MaxRetry:   maxRetry,
		Headers:    cloneHeaders(task.Headers()),
	}
	ctx = queue.ContextWithMessage(ctx, msg)
	ctx = contextWithMessageFields(ctx, msg)

	ctx, span := startWorkerSpan(ctx, msg)
	defer func() {
		if recovered := recover(); recovered != nil {
			span.RecordError(fmt.Errorf("panic: %v", recovered))
			span.SetStatus(tracing.StatusError, "panic")
			span.End()
			panic(recovered)
		}
		if err != nil {
			span.RecordError(err)
			span.SetStatus(tracing.StatusError, err.Error())
		}
		span.End()
	}()

	return handler(ctx, msg)
}

func asynqRetryDelay() hibasynq.RetryDelayFunc {
	return func(n int, err error, task *hibasynq.Task) time.Duration {
		var rateLimitErr *queue.RateLimitError
		if errors.As(err, &rateLimitErr) && rateLimitErr.RetryIn > 0 {
			return rateLimitErr.RetryIn
		}
		if runtimeErr, ok := queue.RuntimeErrorFrom(err); ok && runtimeErr.RetryIn > 0 {
			return runtimeErr.RetryIn
		}
		return hibasynq.DefaultRetryDelayFunc(n, err, task)
	}
}

func taskHeadersFromContext(ctx context.Context) map[string]string {
	return queue.CorrelationHeadersFromContext(ctx)
}

func contextFromTaskHeaders(ctx context.Context, headers map[string]string) context.Context {
	return queue.ContextFromCorrelationHeaders(ctx, headers)
}

func startWorkerSpan(ctx context.Context, msg *queue.Message) (context.Context, tracing.Span) {
	attrs := []tracing.Attr{
		tracing.String("messaging.system", "asynq"),
		tracing.String("messaging.operation", "process"),
	}
	spanName := "consumer::task"
	if msg != nil {
		spanName = "consumer::" + msg.Type
		attrs = append(attrs,
			tracing.String("messaging.destination.name", msg.Queue),
			tracing.String("messaging.message.id", msg.ID),
			tracing.String("messaging.message.type", msg.Type),
			tracing.Int("messaging.message.retry_count", msg.RetryCount),
			tracing.Int("messaging.message.max_retry", msg.MaxRetry),
		)
	}
	ctx, span := tracing.StartSpanWithOptions(ctx, spanName, tracing.SpanOptions{
		TracerName: "sdkitgo/core/queue",
		Kind:       tracing.SpanKindConsumer,
	}, attrs...)
	setQueueCorrelationAttributes(ctx, span)
	return ctx, span
}

func startEnqueueSpan(ctx context.Context, taskType string, opts queue.EnqueueOptions) (context.Context, tracing.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	attrs := []tracing.Attr{
		tracing.String("messaging.system", "asynq"),
		tracing.String("messaging.operation", "publish"),
		tracing.String("messaging.destination.name", opts.Queue),
		tracing.String("messaging.message.id", opts.TaskID),
		tracing.String("messaging.message.type", taskType),
	}
	ctx, span := tracing.StartSpanWithOptions(ctx, "producer::"+taskType, tracing.SpanOptions{
		TracerName: "sdkitgo/core/queue",
		Kind:       tracing.SpanKindProducer,
	}, attrs...)
	setQueueCorrelationAttributes(ctx, span)
	return ctx, span
}

func setQueueCorrelationAttributes(ctx context.Context, span tracing.Span) {
	queue.SetSpanCorrelationAttributes(ctx, span)
}

func contextWithMessageFields(ctx context.Context, msg *queue.Message) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if msg == nil {
		return ctx
	}
	if msg.ID != "" {
		ctx = logger.WithField(ctx, logger.TaskIDKey, msg.ID)
	}
	if msg.Queue != "" {
		ctx = logger.WithField(ctx, logger.QueueKey, msg.Queue)
	}
	if msg.Type != "" {
		ctx = logger.WithField(ctx, logger.TypeKey, msg.Type)
	}
	return ctx
}
