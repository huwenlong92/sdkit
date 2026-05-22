package asynq

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/huwenlong92/sdkit/core/logger"
	corequeue "github.com/huwenlong92/sdkit/core/queue"

	hibasynq "github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type Queue struct {
	cfg       corequeue.Config
	runtime   corequeue.RuntimeOptions
	client    *hibasynq.Client
	server    *hibasynq.Server
	mux       *hibasynq.ServeMux
	inspector *hibasynq.Inspector
	mws       []corequeue.Middleware
}

func New(cfg corequeue.Config, opts ...corequeue.RuntimeOption) *Queue {
	cfg = cfg.Normalize()
	runtime := corequeue.ApplyRuntimeOptions(opts)
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

func (q *Queue) Supports(cap corequeue.Capability) bool {
	return capabilities()[cap]
}

func (q *Queue) Capabilities() map[corequeue.Capability]bool {
	return corequeue.CloneCapabilities(capabilities())
}

func (q *Queue) Enqueue(ctx context.Context, task corequeue.Task, opts ...corequeue.Option) (*corequeue.TaskInfo, error) {
	if q == nil || q.client == nil {
		return nil, corequeue.ErrNotInitialized
	}
	if task.Type == "" {
		return nil, fmt.Errorf("queue: task type is required")
	}
	payload, err := corequeue.MarshalPayload(task.Payload)
	if err != nil {
		return nil, err
	}
	applied := corequeue.ApplyOptions(opts)
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
			span.SetStatus(codes.Error, err.Error())
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
			attribute.String("messaging.message.id", out.ID),
			attribute.String("messaging.destination.name", out.Queue),
		)
	}
	return out, nil
}

func (q *Queue) BatchEnqueue(ctx context.Context, tasks []corequeue.Task, opts ...corequeue.Option) ([]*corequeue.TaskInfo, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	out := make([]*corequeue.TaskInfo, 0, len(tasks))
	for _, task := range tasks {
		info, err := q.Enqueue(ctx, task, opts...)
		if err != nil {
			return out, err
		}
		out = append(out, info)
	}
	return out, nil
}

func (q *Queue) Handle(pattern string, handler corequeue.HandlerFunc) {
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

func (q *Queue) Use(middlewares ...corequeue.Middleware) {
	if q == nil {
		return
	}
	q.mws = append(q.mws, middlewares...)
}

func handleAsynqTask(ctx context.Context, task *hibasynq.Task, handler corequeue.HandlerFunc) (err error) {
	ctx = contextFromTaskHeaders(ctx, task.Headers())

	id, _ := hibasynq.GetTaskID(ctx)
	queueName, _ := hibasynq.GetQueueName(ctx)
	retried, _ := hibasynq.GetRetryCount(ctx)
	maxRetry, _ := hibasynq.GetMaxRetry(ctx)
	msg := &corequeue.Message{
		ID:         id,
		Type:       task.Type(),
		Payload:    task.Payload(),
		Queue:      queueName,
		RetryCount: retried,
		MaxRetry:   maxRetry,
		Headers:    cloneHeaders(task.Headers()),
	}
	ctx = corequeue.ContextWithMessage(ctx, msg)
	ctx = contextWithMessageFields(ctx, msg)

	ctx, span := startWorkerSpan(ctx, msg)
	defer func() {
		if recovered := recover(); recovered != nil {
			span.RecordError(fmt.Errorf("panic: %v", recovered))
			span.SetStatus(codes.Error, "panic")
			span.End()
			panic(recovered)
		}
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	return handler(ctx, msg)
}

func asynqRetryDelay() hibasynq.RetryDelayFunc {
	return func(n int, err error, task *hibasynq.Task) time.Duration {
		var rateLimitErr *corequeue.RateLimitError
		if errors.As(err, &rateLimitErr) && rateLimitErr.RetryIn > 0 {
			return rateLimitErr.RetryIn
		}
		if runtimeErr, ok := corequeue.RuntimeErrorFrom(err); ok && runtimeErr.RetryIn > 0 {
			return runtimeErr.RetryIn
		}
		return hibasynq.DefaultRetryDelayFunc(n, err, task)
	}
}

func taskHeadersFromContext(ctx context.Context) map[string]string {
	return corequeue.CorrelationHeadersFromContext(ctx)
}

func contextFromTaskHeaders(ctx context.Context, headers map[string]string) context.Context {
	return corequeue.ContextFromCorrelationHeaders(ctx, headers)
}

func startWorkerSpan(ctx context.Context, msg *corequeue.Message) (context.Context, oteltrace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String("messaging.system", "asynq"),
		attribute.String("messaging.operation", "process"),
	}
	spanName := "consumer::task"
	if msg != nil {
		spanName = "consumer::" + msg.Type
		attrs = append(attrs,
			attribute.String("messaging.destination.name", msg.Queue),
			attribute.String("messaging.message.id", msg.ID),
			attribute.String("messaging.message.type", msg.Type),
			attribute.Int("messaging.message.retry_count", msg.RetryCount),
			attribute.Int("messaging.message.max_retry", msg.MaxRetry),
		)
	}
	ctx, span := otel.Tracer("sdkitgo/core/queue").Start(ctx, spanName,
		oteltrace.WithSpanKind(oteltrace.SpanKindConsumer),
		oteltrace.WithAttributes(attrs...),
	)
	setQueueCorrelationAttributes(ctx, span)
	return ctx, span
}

func startEnqueueSpan(ctx context.Context, taskType string, opts corequeue.EnqueueOptions) (context.Context, oteltrace.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	attrs := []attribute.KeyValue{
		attribute.String("messaging.system", "asynq"),
		attribute.String("messaging.operation", "publish"),
		attribute.String("messaging.destination.name", opts.Queue),
		attribute.String("messaging.message.id", opts.TaskID),
		attribute.String("messaging.message.type", taskType),
	}
	ctx, span := otel.Tracer("sdkitgo/core/queue").Start(ctx, "producer::"+taskType,
		oteltrace.WithSpanKind(oteltrace.SpanKindProducer),
		oteltrace.WithAttributes(attrs...),
	)
	setQueueCorrelationAttributes(ctx, span)
	return ctx, span
}

func setQueueCorrelationAttributes(ctx context.Context, span oteltrace.Span) {
	corequeue.SetSpanCorrelationAttributes(ctx, span)
}

func contextWithMessageFields(ctx context.Context, msg *corequeue.Message) context.Context {
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
