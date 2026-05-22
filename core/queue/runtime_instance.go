package queue

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	coreruntime "github.com/huwenlong92/sdkit/core/runtime"
)

const RuntimeCapabilityName = "queue.runtime"

type RuntimeParts struct {
	Runner  QueueRunner
	Client  Client
	Worker  Worker
	Manager Manager
}

type RuntimeInstanceOption func(*RuntimeInstance)

type RuntimeInstance struct {
	runner     QueueRunner
	client     Client
	worker     Worker
	manager    Manager
	kernel     *RuntimeKernel
	registry   *RegistryRuntime
	operations *OperationsRuntime
	metadata   RuntimeMetadata
	taskStore  TaskStore

	cancelSchedulePoller context.CancelFunc
}

type runtimeContextKey struct{}

func NewRuntimeInstance(runner QueueRunner, opts ...RuntimeInstanceOption) *RuntimeInstance {
	return NewRuntimeInstanceFromParts(RuntimeParts{
		Runner:  runner,
		Client:  runner,
		Worker:  runner,
		Manager: runner,
	}, opts...)
}

func NewRuntimeInstanceFromParts(parts RuntimeParts, opts ...RuntimeInstanceOption) *RuntimeInstance {
	if parts.Runner != nil {
		if parts.Client == nil {
			parts.Client = parts.Runner
		}
		if parts.Worker == nil {
			parts.Worker = parts.Runner
		}
		if parts.Manager == nil {
			parts.Manager = parts.Runner
		}
	}
	rt := &RuntimeInstance{
		runner:  parts.Runner,
		client:  parts.Client,
		worker:  parts.Worker,
		manager: parts.Manager,
	}
	rt.registry = NewRegistryRuntime(nil)
	rt.operations = NewOperationsRuntime(parts.Manager)
	for _, opt := range opts {
		if opt != nil {
			opt(rt)
		}
	}
	if rt.registry == nil {
		rt.registry = NewRegistryRuntime(nil)
	}
	rt.attachKernelOrchestrator()
	if rt.operations == nil {
		rt.operations = NewOperationsRuntime(rt.manager)
	}
	rt.operations.SetManager(rt.manager)
	rt.operations.SetMetadata(rt.metadata)
	return rt
}

func InitRuntimeInstance(ctx context.Context, cfg Config, runtimeCfg RuntimeKernelConfig, opts ...RuntimeInstanceOption) (*RuntimeInstance, error) {
	kernel, err := InitRuntimeKernel(ctx, cfg, runtimeCfg)
	if err != nil {
		return nil, err
	}
	options := []RuntimeInstanceOption{
		WithRuntimeKernel(kernel),
		WithRuntimeMetadata(RuntimeMetadataFromConfig("", "", cfg)),
	}
	options = append(options, opts...)
	return NewRuntimeInstance(kernel.Runner(), options...), nil
}

func WithRuntimeKernel(kernel *RuntimeKernel) RuntimeInstanceOption {
	return func(rt *RuntimeInstance) {
		if rt == nil {
			return
		}
		rt.kernel = kernel
		if kernel != nil && kernel.Runner() != nil {
			rt.runner = kernel.Runner()
			rt.client = kernel.Runner()
			rt.worker = kernel.Runner()
			rt.manager = kernel.Runner()
			rt.attachKernelOrchestrator()
			if rt.operations != nil {
				rt.operations.SetManager(rt.manager)
			}
		}
	}
}

func WithRuntimeRegistry(registry *RegistryRuntime) RuntimeInstanceOption {
	return func(rt *RuntimeInstance) {
		if rt != nil && registry != nil {
			rt.registry = registry
			rt.attachKernelOrchestrator()
		}
	}
}

func WithRuntimeOperations(operations *OperationsRuntime) RuntimeInstanceOption {
	return func(rt *RuntimeInstance) {
		if rt != nil && operations != nil {
			rt.operations = operations
		}
	}
}

func WithRuntimeMetadata(metadata RuntimeMetadata) RuntimeInstanceOption {
	return func(rt *RuntimeInstance) {
		if rt != nil {
			rt.metadata = cloneRuntimeMetadata(metadata)
			if rt.operations != nil {
				rt.operations.SetMetadata(rt.metadata)
			}
		}
	}
}

func WithRuntimeTaskStore(store TaskStore) RuntimeInstanceOption {
	return func(rt *RuntimeInstance) {
		if rt != nil {
			rt.taskStore = store
		}
	}
}

// From resolves a runtime instance from runtime wiring objects. Business code
// should prefer Runtime(ctx), package helpers, or explicit Client/Manager values.
func From(source any) *RuntimeInstance {
	switch v := source.(type) {
	case nil:
		return nil
	case *RuntimeInstance:
		return v
	case interface{ Container() *coreruntime.Container }:
		if v.Container() == nil {
			return nil
		}
		value, ok := v.Container().Get(coreruntime.Key(RuntimeCapabilityName))
		if !ok {
			return nil
		}
		runtime, _ := value.(*RuntimeInstance)
		return runtime
	default:
		return nil
	}
}

func ContextWithRuntime(ctx context.Context, runtime *RuntimeInstance) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if runtime == nil {
		return ctx
	}
	return context.WithValue(ctx, runtimeContextKey{}, runtime)
}

func Runtime(ctx context.Context) *RuntimeInstance {
	if ctx == nil {
		return nil
	}
	runtime, _ := ctx.Value(runtimeContextKey{}).(*RuntimeInstance)
	return runtime
}

func (rt *RuntimeInstance) Runner() QueueRunner {
	if rt == nil {
		return nil
	}
	return rt.runner
}

func (rt *RuntimeInstance) Client() Client {
	if rt == nil {
		return nil
	}
	return rt.client
}

func (rt *RuntimeInstance) Worker() Worker {
	if rt == nil {
		return nil
	}
	return rt.worker
}

func (rt *RuntimeInstance) Manager() Manager {
	if rt == nil {
		return nil
	}
	return rt.manager
}

func (rt *RuntimeInstance) Kernel() *RuntimeKernel {
	if rt == nil {
		return nil
	}
	return rt.kernel
}

func (rt *RuntimeInstance) Registry() *RegistryRuntime {
	if rt == nil {
		return nil
	}
	return rt.registry
}

func (rt *RuntimeInstance) NewRegistry() *Registry {
	if rt == nil {
		return nil
	}
	if rt.worker == nil {
		return NewRegistryWithRuntime(nil, rt.registry)
	}
	return NewRegistryWithRuntime(runtimeBoundWorker{runtime: rt, worker: rt.worker}, rt.registry)
}

func (rt *RuntimeInstance) Operations() *OperationsRuntime {
	if rt == nil {
		return nil
	}
	return rt.operations
}

func (rt *RuntimeInstance) Metadata() RuntimeMetadata {
	if rt == nil {
		return RuntimeMetadata{}
	}
	return cloneRuntimeMetadata(rt.metadata)
}

func (rt *RuntimeInstance) Enqueue(ctx context.Context, task Task, opts ...Option) (*TaskInfo, error) {
	if rt == nil || rt.client == nil {
		return nil, ErrNotInitialized
	}
	info, scheduled, err := rt.recordScheduled(ctx, task, opts...)
	if err != nil {
		return nil, err
	}
	if scheduled {
		return info, nil
	}
	task, opts, pendingRecord, prepared, err := rt.recordSubmitting(ctx, task, opts...)
	if err != nil {
		return nil, err
	}
	info, err = rt.client.Enqueue(ctx, task, opts...)
	if err != nil {
		if prepared {
			pendingRecord.LastError = err.Error()
			_ = RecordTaskDispatchFailed(ctx, rt.taskStore, pendingRecord)
		}
		return nil, err
	}
	rt.recordEnqueued(ctx, task, info, pendingRecord, prepared, opts...)
	return info, nil
}

func (rt *RuntimeInstance) RequeueTaskRecord(ctx context.Context, record TaskRecord) (*TaskInfo, error) {
	if rt == nil || rt.client == nil {
		return nil, ErrNotInitialized
	}
	queueName := taskStoreFirstNonEmpty(record.Queue, DefaultQueueName)
	if record.TaskID != "" && rt.manager != nil {
		if err := rt.operationsRuntime().DeleteTask(ctx, queueName, record.TaskID); err != nil &&
			!errors.Is(err, ErrTaskNotFound) &&
			!errors.Is(err, ErrCapabilityUnsupported) {
			return nil, err
		}
	}
	record.NextRetryAt = nil
	record.LastRetryError = ""
	task := Task{
		ID:      record.TaskID,
		Type:    record.Type,
		Queue:   queueName,
		Payload: record.Payload,
		Headers: record.Headers,
	}
	opts := taskRecordOptions(record)
	info, err := rt.client.Enqueue(ctx, task, opts...)
	if err != nil {
		record.LastError = err.Error()
		_ = RecordTaskDispatchFailed(ctx, rt.taskStore, record)
		return nil, err
	}
	rt.recordEnqueued(ctx, task, info, record, true, opts...)
	return info, nil
}

func (rt *RuntimeInstance) DispatchAutoRetryTasks(ctx context.Context, limit int) (int, error) {
	if rt == nil || rt.client == nil || rt.taskStore == nil {
		return 0, nil
	}
	store, ok := rt.taskStore.(TaskAutoRetryStore)
	if !ok || store == nil {
		return 0, nil
	}
	records, err := store.ClaimAutoRetryTasks(ctx, time.Now(), limit, rt.scheduleWorkerID(), rt.Metadata().Driver)
	if err != nil {
		return 0, err
	}
	dispatched := 0
	var dispatchErr error
	for _, record := range records {
		if _, err := rt.RequeueTaskRecord(ctx, record); err != nil {
			_ = store.MarkAutoRetryFailed(ctx, record, err)
			dispatchErr = errors.Join(dispatchErr, err)
			continue
		}
		dispatched++
	}
	return dispatched, dispatchErr
}

func (rt *RuntimeInstance) BatchEnqueue(ctx context.Context, tasks []Task, opts ...Option) ([]*TaskInfo, error) {
	if rt == nil || rt.client == nil {
		return nil, ErrNotInitialized
	}
	if rt.taskStore != nil {
		if _, ok := rt.taskStore.(TaskSubmissionStore); ok {
			infos := make([]*TaskInfo, 0, len(tasks))
			for _, task := range tasks {
				info, err := rt.Enqueue(ctx, task, opts...)
				if err != nil {
					return infos, err
				}
				infos = append(infos, info)
			}
			return infos, nil
		}
	}
	infos, err := rt.client.BatchEnqueue(ctx, tasks, opts...)
	if err != nil {
		return infos, err
	}
	for i, task := range tasks {
		var info *TaskInfo
		if i < len(infos) {
			info = infos[i]
		}
		rt.recordEnqueued(ctx, task, info, TaskRecord{}, false, opts...)
	}
	return infos, nil
}

func (rt *RuntimeInstance) recordScheduled(ctx context.Context, task Task, opts ...Option) (*TaskInfo, bool, error) {
	if rt == nil || rt.taskStore == nil {
		return nil, false, nil
	}
	if _, ok := rt.taskStore.(TaskScheduleStore); !ok {
		return nil, false, nil
	}
	applied := ApplyOptions(opts)
	if task.Queue != "" {
		applied.Queue = task.Queue
	}
	if task.ID != "" {
		applied.TaskID = task.ID
	}
	scheduledAt, ok := enqueueScheduledAt(applied)
	if !ok || !scheduledAt.After(time.Now()) {
		return nil, false, nil
	}
	if applied.TaskID == "" {
		applied.TaskID = uuid.NewString()
		task.ID = applied.TaskID
	}
	record, err := rt.taskRecordFromEnqueue(ctx, task, nil, applied, StateScheduled)
	if err != nil {
		return nil, false, err
	}
	record.ScheduledAt = &scheduledAt
	record, err = RecordTaskScheduled(ctx, rt.taskStore, record)
	if err != nil {
		return nil, false, err
	}
	return &TaskInfo{
		ID:        record.TaskID,
		Type:      record.Type,
		Queue:     record.Queue,
		State:     StateScheduled,
		Payload:   record.Payload,
		Headers:   record.Headers,
		MaxRetry:  record.MaxRetry,
		NextRunAt: record.ScheduledAt,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
		TrackID:   record.TrackID,
		RequestID: record.RequestID,
		TraceID:   record.TraceID,
		SpanID:    record.SpanID,
	}, true, nil
}

func (rt *RuntimeInstance) recordSubmitting(ctx context.Context, task Task, opts ...Option) (Task, []Option, TaskRecord, bool, error) {
	if rt == nil || rt.taskStore == nil {
		return task, opts, TaskRecord{}, false, nil
	}
	if _, ok := rt.taskStore.(TaskSubmissionStore); !ok {
		return task, opts, TaskRecord{}, false, nil
	}
	applied := ApplyOptions(opts)
	if task.Queue != "" {
		applied.Queue = task.Queue
	}
	if task.ID != "" {
		applied.TaskID = task.ID
	}
	if applied.TaskID == "" {
		applied.TaskID = uuid.NewString()
		task.ID = applied.TaskID
		opts = append(opts, TaskID(applied.TaskID))
	}
	record, err := rt.taskRecordFromEnqueue(ctx, task, nil, applied, StateSubmitting)
	if err != nil {
		return task, opts, TaskRecord{}, false, err
	}
	record, err = RecordTaskSubmitting(ctx, rt.taskStore, record)
	if err != nil {
		return task, opts, TaskRecord{}, false, err
	}
	return task, opts, record, true, nil
}

func (rt *RuntimeInstance) recordEnqueued(ctx context.Context, task Task, info *TaskInfo, pendingRecord TaskRecord, prepared bool, opts ...Option) {
	if rt == nil || rt.taskStore == nil || info == nil {
		return
	}
	applied := ApplyOptions(opts)
	if task.Queue != "" {
		applied.Queue = task.Queue
	}
	if task.ID != "" {
		applied.TaskID = task.ID
	}
	state := info.State
	if state == "" {
		state = StatePending
	}
	record, err := rt.taskRecordFromEnqueue(ctx, task, info, applied, state)
	if err != nil {
		return
	}
	if prepared {
		record.RecordID = pendingRecord.RecordID
		record.CreatedAt = pendingRecord.CreatedAt
		record.AutoRetryEnabled = pendingRecord.AutoRetryEnabled
		record.AutoRetryCount = pendingRecord.AutoRetryCount
		record.AutoRetryMax = pendingRecord.AutoRetryMax
		record.AutoRetryDelaySeconds = pendingRecord.AutoRetryDelaySeconds
		record.NextRetryAt = pendingRecord.NextRetryAt
		record.LastRetryError = pendingRecord.LastRetryError
		_ = RecordTaskDispatched(ctx, rt.taskStore, record)
		return
	}
	_ = RecordTaskEnqueued(ctx, rt.taskStore, record)
}

func (rt *RuntimeInstance) taskRecordFromEnqueue(ctx context.Context, task Task, info *TaskInfo, applied EnqueueOptions, state TaskState) (TaskRecord, error) {
	payload, err := MarshalPayload(task.Payload)
	if err != nil {
		return TaskRecord{}, err
	}
	headers := CorrelationHeadersFromContext(ctx)
	for k, v := range task.Headers {
		if headers == nil {
			headers = map[string]string{}
		}
		headers[k] = v
	}
	var scheduledAt *time.Time
	if info != nil {
		scheduledAt = info.NextRunAt
	}
	delaySeconds := 0
	if !applied.ProcessAt.IsZero() {
		scheduledAt = &applied.ProcessAt
		delaySeconds = int(time.Until(applied.ProcessAt).Seconds())
	} else if applied.ProcessIn > 0 {
		at := time.Now().Add(applied.ProcessIn)
		scheduledAt = &at
		delaySeconds = int(applied.ProcessIn.Seconds())
	}
	if scheduledAt != nil && scheduledAt.After(time.Now()) && state == StatePending {
		state = StateScheduled
	}
	maxRetry := 0
	if info != nil {
		maxRetry = info.MaxRetry
	}
	if applied.MaxRetry != nil {
		maxRetry = *applied.MaxRetry
	}
	createdAt := time.Now()
	if info != nil && !info.CreatedAt.IsZero() {
		createdAt = info.CreatedAt
	}
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	updatedAt := createdAt
	if info != nil && !info.UpdatedAt.IsZero() {
		updatedAt = info.UpdatedAt
	}
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	taskID := applied.TaskID
	if info != nil && info.ID != "" {
		taskID = info.ID
	}
	return TaskRecord{
		Driver:                rt.metadata.Driver,
		TaskID:                taskID,
		Queue:                 taskStoreFirstNonEmpty(task.Queue, applied.Queue, DefaultQueueName),
		Type:                  task.Type,
		Payload:               payload,
		State:                 state,
		MaxRetry:              maxRetry,
		TimeoutSeconds:        int(applied.Timeout.Seconds()),
		UniqueSeconds:         int(applied.UniqueTTL.Seconds()),
		DelaySeconds:          delaySeconds,
		AutoRetryEnabled:      applied.AutoRetryEnabled,
		AutoRetryMax:          applied.AutoRetryMax,
		AutoRetryDelaySeconds: int(applied.AutoRetryDelay.Seconds()),
		ScheduledAt:           scheduledAt,
		CreatedAt:             createdAt,
		UpdatedAt:             updatedAt,
		Headers:               headers,
	}, nil
}

func taskRecordOptions(record TaskRecord) []Option {
	opts := []Option{Queue(taskStoreFirstNonEmpty(record.Queue, DefaultQueueName))}
	if record.TaskID != "" {
		opts = append(opts, TaskID(record.TaskID))
	}
	opts = append(opts, MaxRetry(record.MaxRetry))
	if record.TimeoutSeconds > 0 {
		opts = append(opts, Timeout(time.Duration(record.TimeoutSeconds)*time.Second))
	}
	if record.UniqueSeconds > 0 {
		opts = append(opts, Unique(time.Duration(record.UniqueSeconds)*time.Second))
	}
	if record.AutoRetryEnabled && record.AutoRetryMax > 0 {
		opts = append(opts, AutoRetry(record.AutoRetryMax, time.Duration(record.AutoRetryDelaySeconds)*time.Second))
	}
	return opts
}

func enqueueScheduledAt(applied EnqueueOptions) (time.Time, bool) {
	if !applied.ProcessAt.IsZero() {
		return applied.ProcessAt, true
	}
	if applied.ProcessIn > 0 {
		return time.Now().Add(applied.ProcessIn), true
	}
	return time.Time{}, false
}

func (rt *RuntimeInstance) Close() error {
	if rt == nil {
		return nil
	}
	if rt.cancelSchedulePoller != nil {
		rt.cancelSchedulePoller()
		rt.cancelSchedulePoller = nil
	}
	if rt.kernel != nil {
		rt.kernel.Close()
	}
	closed := make([]any, 0, 3)
	closeOnce := func(instance any) error {
		if instance == nil {
			return nil
		}
		for _, item := range closed {
			if sameInstance(item, instance) {
				return nil
			}
		}
		closer, ok := instance.(interface{ Close() error })
		if !ok {
			return nil
		}
		closed = append(closed, instance)
		return closer.Close()
	}
	return errors.Join(
		closeOnce(rt.runner),
		closeOnce(rt.client),
		closeOnce(rt.manager),
	)
}

func (rt *RuntimeInstance) StartSchedulePoller(ctx context.Context, cfg ScheduleConfig) context.CancelFunc {
	if rt == nil || rt.client == nil || rt.taskStore == nil || !cfg.Enabled {
		return nil
	}
	store, ok := rt.taskStore.(TaskScheduleStore)
	if !ok || store == nil {
		return nil
	}
	cfg = normalizeConfig(Config{Schedule: cfg}).Schedule
	if ctx == nil {
		ctx = context.Background()
	}
	pollerCtx, cancel := context.WithCancel(ctx)
	rt.cancelSchedulePoller = cancel
	go rt.runSchedulePoller(pollerCtx, store, cfg)
	return cancel
}

func (rt *RuntimeInstance) runSchedulePoller(ctx context.Context, store TaskScheduleStore, cfg ScheduleConfig) {
	rt.dispatchScheduledTasks(ctx, store, cfg)
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rt.dispatchScheduledTasks(ctx, store, cfg)
		}
	}
}

func (rt *RuntimeInstance) dispatchScheduledTasks(ctx context.Context, store TaskScheduleStore, cfg ScheduleConfig) {
	records, err := store.ClaimScheduled(ctx, time.Now(), cfg.BatchSize, rt.scheduleWorkerID(), rt.Metadata().Driver)
	if err != nil || len(records) == 0 {
		return
	}
	for _, record := range records {
		rt.dispatchScheduledTask(ctx, record)
	}
}

func (rt *RuntimeInstance) dispatchScheduledTask(ctx context.Context, record TaskRecord) {
	if rt == nil || rt.client == nil {
		return
	}
	task := Task{
		ID:      record.TaskID,
		Type:    record.Type,
		Queue:   record.Queue,
		Payload: record.Payload,
		Headers: record.Headers,
	}
	opts := taskRecordOptions(record)
	info, err := rt.client.Enqueue(ctx, task, opts...)
	if err != nil {
		record.LastError = err.Error()
		_ = RecordTaskDispatchFailed(ctx, rt.taskStore, record)
		return
	}
	rt.recordEnqueued(ctx, task, info, record, true, opts...)
}

func (rt *RuntimeInstance) scheduleWorkerID() string {
	if rt == nil {
		return ""
	}
	metadata := rt.Metadata()
	return taskStoreFirstNonEmpty(metadata.WorkerMetadata.ID, metadata.WorkerMetadata.Name, metadata.Worker, metadata.Name)
}

func (rt *RuntimeInstance) Handle(pattern string, handler HandlerFunc) {
	if rt == nil || rt.worker == nil || handler == nil {
		return
	}
	registry := rt.NewRegistry()
	if registry == nil {
		return
	}
	_ = registry.Register(pattern, handler)
}

func (rt *RuntimeInstance) Use(middlewares ...Middleware) {
	if rt == nil || rt.registry == nil {
		return
	}
	rt.registry.Use(middlewares...)
}

func (rt *RuntimeInstance) AddHook(h Hook) {
	if rt == nil || rt.registry == nil {
		return
	}
	rt.registry.AddHook(h)
}

func (rt *RuntimeInstance) UseRuntime(middlewares ...RuntimeMiddleware) {
	if rt == nil || rt.registry == nil {
		return
	}
	rt.registry.UseRuntime(middlewares...)
}

func (rt *RuntimeInstance) Run(ctx context.Context) error {
	if rt == nil || rt.worker == nil {
		return ErrNotInitialized
	}
	ctx = ContextWithRuntime(ctx, rt)
	return rt.worker.Run(ctx)
}

func (rt *RuntimeInstance) Shutdown(ctx context.Context) error {
	if rt == nil || rt.worker == nil {
		return nil
	}
	return rt.worker.Shutdown(ContextWithRuntime(ctx, rt))
}

func (rt *RuntimeInstance) Supports(cap Capability) bool {
	if rt == nil || rt.manager == nil {
		return false
	}
	return rt.manager.Supports(cap)
}

func (rt *RuntimeInstance) Capabilities() map[Capability]bool {
	if rt == nil || rt.manager == nil {
		return nil
	}
	return rt.manager.Capabilities()
}

func (rt *RuntimeInstance) ListQueues(ctx context.Context) ([]*QueueInfo, error) {
	return rt.operationsRuntime().ListQueues(ctx)
}

func (rt *RuntimeInstance) GetQueue(ctx context.Context, queueName string) (*QueueInfo, error) {
	return rt.operationsRuntime().GetQueue(ctx, queueName)
}

func (rt *RuntimeInstance) ListTasks(ctx context.Context, query TaskQuery) ([]*TaskInfo, error) {
	return rt.operationsRuntime().ListTasks(ctx, query)
}

func (rt *RuntimeInstance) GetTask(ctx context.Context, queueName string, taskID string) (*TaskInfo, error) {
	return rt.operationsRuntime().GetTask(ctx, queueName, taskID)
}

func (rt *RuntimeInstance) DeleteTask(ctx context.Context, queueName string, taskID string) error {
	return rt.operationsRuntime().DeleteTask(ctx, queueName, taskID)
}

func (rt *RuntimeInstance) RetryTask(ctx context.Context, queueName string, taskID string) error {
	return rt.operationsRuntime().RetryTask(ctx, queueName, taskID)
}

func (rt *RuntimeInstance) ArchiveTask(ctx context.Context, queueName string, taskID string) error {
	return rt.operationsRuntime().ArchiveTask(ctx, queueName, taskID)
}

func (rt *RuntimeInstance) CancelTask(ctx context.Context, queueName string, taskID string) error {
	return rt.operationsRuntime().CancelTask(ctx, queueName, taskID)
}

func (rt *RuntimeInstance) PauseQueue(ctx context.Context, queueName string) error {
	return rt.operationsRuntime().PauseQueue(ctx, queueName)
}

func (rt *RuntimeInstance) ResumeQueue(ctx context.Context, queueName string) error {
	return rt.operationsRuntime().ResumeQueue(ctx, queueName)
}

func (rt *RuntimeInstance) operationsRuntime() *OperationsRuntime {
	if rt == nil || rt.operations == nil {
		return NewOperationsRuntime(nil)
	}
	return rt.operations
}

func (rt *RuntimeInstance) attachKernelOrchestrator() {
	if rt == nil || rt.registry == nil || rt.kernel == nil {
		return
	}
	if orchestrator := rt.kernel.Orchestrator(); orchestrator != nil {
		rt.registry.SetOrchestrator(orchestrator)
	}
}

var _ QueueRunner = (*RuntimeInstance)(nil)

type runtimeBoundWorker struct {
	runtime *RuntimeInstance
	worker  Worker
}

func (w runtimeBoundWorker) Handle(pattern string, handler HandlerFunc) {
	if w.worker == nil || handler == nil {
		return
	}
	w.worker.Handle(pattern, func(ctx context.Context, msg *Message) error {
		return handler(ContextWithRuntime(ctx, w.runtime), msg)
	})
}

func (w runtimeBoundWorker) Use(middlewares ...Middleware) {
	if w.worker == nil {
		return
	}
	w.worker.Use(middlewares...)
}

func (w runtimeBoundWorker) Run(ctx context.Context) error {
	if w.worker == nil {
		return ErrNotInitialized
	}
	return w.worker.Run(ContextWithRuntime(ctx, w.runtime))
}

func (w runtimeBoundWorker) Shutdown(ctx context.Context) error {
	if w.worker == nil {
		return nil
	}
	return w.worker.Shutdown(ContextWithRuntime(ctx, w.runtime))
}
