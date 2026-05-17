package queue

import (
	"context"
	"reflect"
	goruntime "runtime"
)

type RegistryRuntime struct {
	dispatcher *Dispatcher
}

func NewRegistryRuntime(dispatcher *Dispatcher) *RegistryRuntime {
	if dispatcher == nil {
		dispatcher = NewDispatcher()
	}
	return &RegistryRuntime{dispatcher: dispatcher}
}

func (r *RegistryRuntime) Use(middlewares ...Middleware) {
	if r == nil {
		return
	}
	r.dispatcher.Use(middlewares...)
}

func (r *RegistryRuntime) UseRuntime(middlewares ...RuntimeMiddleware) {
	if r == nil {
		return
	}
	r.dispatcher.UseRuntime(middlewares...)
}

func (r *RegistryRuntime) AddHook(h Hook) {
	if r == nil {
		return
	}
	r.dispatcher.AddHook(h)
}

func (r *RegistryRuntime) SetOrchestrator(orchestrator *Orchestrator) {
	if r == nil || r.dispatcher == nil || orchestrator == nil {
		return
	}
	r.dispatcher.SetOrchestrator(orchestrator)
}

func (r *RegistryRuntime) Register(pattern string, handlers ...any) error {
	if r == nil {
		return ErrNotInitialized
	}
	return r.dispatcher.Register(pattern, handlers...)
}

func (r *RegistryRuntime) Handler(pattern string) HandlerFunc {
	if r == nil {
		return func(ctx context.Context, msg *Message) error {
			return ErrNotInitialized
		}
	}
	return r.dispatcher.Handler(pattern)
}

func (r *RegistryRuntime) Dispatch(ctx context.Context, pattern string, msg *Message) error {
	if r == nil {
		return ErrNotInitialized
	}
	return r.dispatcher.Dispatch(ctx, pattern, msg)
}

func (r *RegistryRuntime) Dispatcher() *Dispatcher {
	if r == nil {
		return nil
	}
	return r.dispatcher
}

func (r *RegistryRuntime) Entries() []HandlerMetadata {
	if r == nil || r.dispatcher == nil {
		return nil
	}
	return r.dispatcher.Entries()
}

func (r *RegistryRuntime) Metadata() RegistryMetadata {
	if r == nil || r.dispatcher == nil {
		return RegistryMetadata{}
	}
	return r.dispatcher.Metadata()
}

func splitRegistrationArgs(args []any) ([]any, []Option) {
	if len(args) == 0 {
		return nil, nil
	}
	handlers := make([]any, 0, len(args))
	opts := make([]Option, 0)
	for _, arg := range args {
		if isNilReflectValue(reflect.ValueOf(arg)) {
			continue
		}
		if opt, ok := arg.(Option); ok {
			opts = append(opts, opt)
			continue
		}
		handlers = append(handlers, arg)
	}
	return handlers, opts
}

func handlerMetadata(pattern string, handlers []any, middlewareCount int, opts []Option) HandlerMetadata {
	metadata := HandlerMetadata{
		Pattern:         pattern,
		MiddlewareCount: middlewareCount,
	}
	applied := ApplyOptions(opts)
	metadata.Queue = applied.Queue
	metadata.MaxRetry = cloneIntPtr(applied.MaxRetry)
	metadata.Timeout = applied.Timeout
	metadata.Delay = applied.ProcessIn
	metadata.Priority = applied.Priority
	metadata.Trace = applied.Trace
	if len(handlers) == 0 {
		return metadata
	}
	final := handlers[len(handlers)-1]
	metadata.Handler = handlerName(final)
	metadata.Payload = payloadTypeName(final)
	return metadata
}

func handlerName(handler any) string {
	if handler == nil {
		return ""
	}
	value := reflect.ValueOf(handler)
	if !value.IsValid() || value.Kind() != reflect.Func {
		return reflect.TypeOf(handler).String()
	}
	fn := goruntime.FuncForPC(value.Pointer())
	if fn == nil {
		return value.Type().String()
	}
	return fn.Name()
}

func payloadTypeName(handler any) string {
	if handler == nil {
		return ""
	}
	value := reflect.ValueOf(handler)
	if !value.IsValid() || value.Kind() != reflect.Func {
		return ""
	}
	handlerType := value.Type()
	if handlerType.NumIn() != 2 {
		return ""
	}
	payloadType := handlerType.In(1)
	if payloadType == reflect.TypeOf((*Message)(nil)) {
		return "queue.Message"
	}
	return payloadType.String()
}
