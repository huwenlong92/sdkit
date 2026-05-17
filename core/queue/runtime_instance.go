package queue

import (
	"context"
	"errors"

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
	return rt.client.Enqueue(ctx, task, opts...)
}

func (rt *RuntimeInstance) BatchEnqueue(ctx context.Context, tasks []Task, opts ...Option) ([]*TaskInfo, error) {
	if rt == nil || rt.client == nil {
		return nil, ErrNotInitialized
	}
	return rt.client.BatchEnqueue(ctx, tasks, opts...)
}

func (rt *RuntimeInstance) Close() error {
	if rt == nil {
		return nil
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
