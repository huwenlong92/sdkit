package queue

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type HandlerMetadata struct {
	Pattern         string
	Handler         string
	Payload         string
	MiddlewareCount int
	Queue           string
	MaxRetry        *int
	Timeout         time.Duration
	Delay           time.Duration
	Priority        int
	Trace           bool
}

type Dispatcher struct {
	mu              sync.RWMutex
	handlers        map[string]HandlerFunc
	routeMiddleware map[string][]RuntimeMiddleware
	entries         map[string]HandlerMetadata
	middlewares     []RuntimeMiddleware
	middlewareNames []string
	hooks           []Hook
	orchestrator    *Orchestrator
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		handlers:        map[string]HandlerFunc{},
		routeMiddleware: map[string][]RuntimeMiddleware{},
		entries:         map[string]HandlerMetadata{},
		orchestrator:    NewOrchestrator(),
	}
}

func (d *Dispatcher) Use(middlewares ...Middleware) {
	d.UseRuntime(RuntimeMiddlewares(BusinessStage, middlewares...)...)
}

func (d *Dispatcher) UseRuntime(middlewares ...RuntimeMiddleware) {
	if d == nil || len(middlewares) == 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, middleware := range middlewares {
		if middleware.Middleware != nil {
			d.middlewares = append(d.middlewares, middleware)
			d.middlewareNames = append(d.middlewareNames, handlerName(middleware.Middleware))
		}
	}
}

func (d *Dispatcher) SetOrchestrator(orchestrator *Orchestrator) {
	if d == nil || orchestrator == nil {
		return
	}
	d.mu.Lock()
	d.orchestrator = orchestrator
	d.mu.Unlock()
}

func (d *Dispatcher) Orchestrator() *Orchestrator {
	if d == nil {
		return nil
	}
	d.mu.RLock()
	orchestrator := d.orchestrator
	d.mu.RUnlock()
	if orchestrator == nil {
		return NewOrchestrator()
	}
	return orchestrator
}

func (d *Dispatcher) AddHook(h Hook) {
	if d == nil || h == nil {
		return
	}
	d.mu.Lock()
	d.hooks = append(d.hooks, h)
	d.mu.Unlock()
}

func (d *Dispatcher) Register(pattern string, handlers ...any) error {
	if d == nil {
		return ErrNotInitialized
	}
	if pattern == "" {
		return fmt.Errorf("queue dispatcher pattern is required")
	}
	chain, opts := splitRegistrationArgs(handlers)
	handler, middlewares, err := buildHandlerPipeline(chain...)
	if err != nil {
		return err
	}
	metadata := handlerMetadata(pattern, chain, len(middlewares), opts)
	if metadata.Timeout > 0 && !hasRuntimeMiddlewareStage(middlewares, TimeoutStage) {
		middlewares = append(middlewares, StageMiddleware(TimeoutStage, taskTimeoutMiddleware(metadata.Timeout)))
		metadata.MiddlewareCount = len(middlewares)
	}
	d.mu.Lock()
	if d.handlers == nil {
		d.handlers = map[string]HandlerFunc{}
	}
	if d.routeMiddleware == nil {
		d.routeMiddleware = map[string][]RuntimeMiddleware{}
	}
	if d.entries == nil {
		d.entries = map[string]HandlerMetadata{}
	}
	d.handlers[pattern] = handler
	d.routeMiddleware[pattern] = cloneRuntimeMiddlewares(middlewares)
	d.entries[pattern] = metadata
	d.mu.Unlock()
	return nil
}

func (d *Dispatcher) Handler(pattern string) HandlerFunc {
	return func(ctx context.Context, msg *Message) error {
		return d.Dispatch(ctx, pattern, msg)
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context, pattern string, msg *Message) error {
	if d == nil {
		return ErrNotInitialized
	}
	if pattern == "" && msg != nil {
		pattern = msg.Type
	}
	if pattern == "" {
		return fmt.Errorf("queue dispatcher pattern is required")
	}
	handler, middlewares, metadata, hooks, orchestrator, ok := d.lookup(pattern)
	if !ok {
		return fmt.Errorf("%w: %s", ErrHandlerNotFound, pattern)
	}
	if msg == nil {
		msg = &Message{Type: pattern}
	} else if msg.Type == "" {
		copied := *msg
		copied.Type = pattern
		msg = &copied
	}
	applyHandlerMetadata(msg, metadata)
	if orchestrator == nil {
		orchestrator = NewOrchestrator()
	}
	return orchestrator.Execute(ctx, RuntimeExecution{
		Pattern:     pattern,
		Handler:     handler,
		Middlewares: middlewares,
		Hooks:       hooks,
		Message:     msg,
		Metadata:    metadata,
	})
}

func (d *Dispatcher) HandlerFor(pattern string) (HandlerFunc, bool) {
	if d == nil {
		return nil, false
	}
	d.mu.RLock()
	_, ok := d.handlers[pattern]
	d.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return d.Handler(pattern), true
}

func (d *Dispatcher) Entries() []HandlerMetadata {
	if d == nil {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]HandlerMetadata, 0, len(d.handlers))
	for pattern := range d.handlers {
		if metadata, ok := d.entries[pattern]; ok {
			out = append(out, metadata)
			continue
		}
		out = append(out, HandlerMetadata{Pattern: pattern})
	}
	return out
}

func (d *Dispatcher) Metadata() RegistryMetadata {
	if d == nil {
		return RegistryMetadata{}
	}
	d.mu.RLock()
	middleware := MiddlewareMetadata{
		Count: len(d.middlewares),
		Names: cloneStrings(d.middlewareNames),
	}
	d.mu.RUnlock()
	return RegistryMetadata{
		Handlers:   d.Entries(),
		Middleware: middleware,
	}
}

func (d *Dispatcher) lookup(pattern string) (HandlerFunc, []RuntimeMiddleware, HandlerMetadata, []Hook, *Orchestrator, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	handler, ok := d.handlers[pattern]
	if !ok {
		return nil, nil, HandlerMetadata{}, nil, nil, false
	}
	middlewares := make([]RuntimeMiddleware, 0, len(d.middlewares)+len(d.routeMiddleware[pattern]))
	middlewares = append(middlewares, d.middlewares...)
	middlewares = append(middlewares, d.routeMiddleware[pattern]...)
	middlewares = normalizeRuntimeMiddlewares(middlewares)
	hooks := make([]Hook, len(d.hooks))
	copy(hooks, d.hooks)
	return handler, middlewares, d.entries[pattern], hooks, d.orchestrator, true
}

func applyHandlerMetadata(msg *Message, metadata HandlerMetadata) {
	if msg == nil {
		return
	}
	SetMessageMetadata(msg, MessageMetadataPattern, metadata.Pattern)
	if metadata.Queue != "" {
		SetMessageMetadata(msg, MessageMetadataQueue, metadata.Queue)
		if msg.Queue == "" {
			msg.Queue = metadata.Queue
		}
	}
	if metadata.MaxRetry != nil {
		SetMessageMetadata(msg, MessageMetadataMaxRetry, *metadata.MaxRetry)
	}
	if metadata.Timeout > 0 {
		SetMessageMetadata(msg, MessageMetadataTimeout, metadata.Timeout)
	}
	if metadata.Delay > 0 {
		SetMessageMetadata(msg, MessageMetadataDelay, metadata.Delay)
	}
	if metadata.Priority != 0 {
		SetMessageMetadata(msg, MessageMetadataPriority, metadata.Priority)
	}
	SetMessageMetadata(msg, MessageMetadataTrace, metadata.Trace)
}

func runBeforeProcess(ctx context.Context, msg *Message, hooks []Hook) error {
	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		if err := hook.BeforeProcess(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

func runAfterProcess(ctx context.Context, msg *Message, err error, hooks []Hook) {
	for _, hook := range hooks {
		if hook != nil {
			hook.AfterProcess(ctx, msg, err)
		}
	}
}

func runOnSuccess(ctx context.Context, msg *Message, hooks []Hook) {
	for _, hook := range hooks {
		if hook != nil {
			hook.OnSuccess(ctx, msg)
		}
	}
}

func runOnFailure(ctx context.Context, msg *Message, err error, hooks []Hook) {
	for _, hook := range hooks {
		if hook != nil {
			hook.OnFailure(ctx, msg, err)
		}
	}
}

func taskTimeoutMiddleware(timeout time.Duration) Middleware {
	if timeout <= 0 {
		return nil
	}
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, msg *Message) error {
			if ctx == nil {
				ctx = context.Background()
			}
			timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return next(timeoutCtx, msg)
		}
	}
}
