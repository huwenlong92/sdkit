package queue

import (
	"context"
	"fmt"
	"reflect"

	"github.com/huwenlong92/sdkit/core/jsonx"
)

var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType   = reflect.TypeOf((*error)(nil)).Elem()
)

func New(cfg Config, opts ...RuntimeOption) QueueRunner {
	q, err := NewRunner(cfg, opts...)
	if err != nil {
		return newUnavailableRunner(err)
	}
	return q
}

func Enqueue(ctx context.Context, task Task, opts ...Option) (*TaskInfo, error) {
	runtime := Runtime(ctx)
	if runtime == nil {
		return nil, ErrNotInitialized
	}
	return runtime.Enqueue(ctx, task, opts...)
}

func RequeueTaskRecord(ctx context.Context, record TaskRecord) (*TaskInfo, error) {
	runtime := Runtime(ctx)
	if runtime == nil {
		return nil, ErrNotInitialized
	}
	return runtime.RequeueTaskRecord(ctx, record)
}

func DispatchAutoRetryTasks(ctx context.Context, limit int) (int, error) {
	runtime := Runtime(ctx)
	if runtime == nil {
		return 0, ErrNotInitialized
	}
	return runtime.DispatchAutoRetryTasks(ctx, limit)
}

func BatchEnqueue(ctx context.Context, tasks []Task, opts ...Option) ([]*TaskInfo, error) {
	runtime := Runtime(ctx)
	if runtime == nil {
		return nil, ErrNotInitialized
	}
	return runtime.BatchEnqueue(ctx, tasks, opts...)
}

func Chain(handler HandlerFunc, middlewares ...Middleware) HandlerFunc {
	return ChainRuntime(handler, RuntimeMiddlewares(BusinessStage, middlewares...)...)
}

func BuildHandlerChain(handlers ...any) (HandlerFunc, error) {
	final, middlewares, err := buildHandlerPipeline(handlers...)
	if err != nil {
		return nil, err
	}
	return ChainRuntime(final, middlewares...), nil
}

func asHandlerFunc(handler any) (HandlerFunc, bool) {
	switch h := handler.(type) {
	case HandlerFunc:
		return func(ctx context.Context, msg *Message) error {
			return h(ContextWithMessage(ctx, msg), msg)
		}, true
	case func(context.Context, *Message) error:
		return func(ctx context.Context, msg *Message) error {
			return h(ContextWithMessage(ctx, msg), msg)
		}, true
	case ContextHandler:
		return func(ctx context.Context, msg *Message) error {
			return newHandlerContext(ctx, msg, []ContextHandler{h}).Next()
		}, true
	default:
		return asTypedPayloadHandler(handler)
	}
}

func asTypedPayloadHandler(handler any) (HandlerFunc, bool) {
	if handler == nil {
		return nil, false
	}
	value := reflect.ValueOf(handler)
	if !value.IsValid() || value.Kind() != reflect.Func {
		return nil, false
	}
	handlerType := value.Type()
	if handlerType.NumIn() != 2 || handlerType.NumOut() != 1 {
		return nil, false
	}
	if !handlerType.In(0).Implements(contextType) || !handlerType.Out(0).Implements(errorType) {
		return nil, false
	}
	payloadType := handlerType.In(1)
	if payloadType.Kind() != reflect.Ptr {
		return nil, false
	}
	return func(ctx context.Context, msg *Message) error {
		if ctx == nil {
			ctx = context.Background()
		}
		ctx = ContextWithMessage(ctx, msg)
		payload := reflect.New(payloadType.Elem())
		if msg != nil && len(msg.Payload) > 0 {
			if err := jsonx.Unmarshal(msg.Payload, payload.Interface()); err != nil {
				return err
			}
		}
		results := value.Call([]reflect.Value{reflect.ValueOf(ctx), payload})
		if len(results) == 0 || isNilReflectValue(results[0]) {
			return nil
		}
		return results[0].Interface().(error)
	}, true
}

func isNilReflectValue(value reflect.Value) bool {
	if !value.IsValid() {
		return true
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func buildHandlerPipeline(handlers ...any) (HandlerFunc, []RuntimeMiddleware, error) {
	if len(handlers) == 0 {
		return nil, nil, fmt.Errorf("queue: handler chain requires final handler")
	}
	final, ok := asHandlerFunc(handlers[len(handlers)-1])
	if !ok {
		return nil, nil, fmt.Errorf("queue: handler chain final item must be queue.HandlerFunc, queue.ContextHandler or typed payload handler")
	}
	middlewares := make([]RuntimeMiddleware, 0, len(handlers)-1)
	for i, handler := range handlers[:len(handlers)-1] {
		middleware, ok := asRuntimeMiddleware(handler)
		if !ok {
			return nil, nil, fmt.Errorf("queue: handler chain item %d must be queue.Middleware, queue.RuntimeMiddleware or queue.ContextHandler", i)
		}
		if middleware.Middleware != nil {
			middlewares = append(middlewares, middleware)
		}
	}
	return final, normalizeRuntimeMiddlewares(middlewares), nil
}

func asRuntimeMiddleware(handler any) (RuntimeMiddleware, bool) {
	switch h := handler.(type) {
	case RuntimeMiddleware:
		return h, true
	case Middleware:
		return BusinessMiddleware(h), true
	case func(HandlerFunc) HandlerFunc:
		return BusinessMiddleware(Middleware(h)), true
	case ContextHandler:
		return BusinessMiddleware(ContextChain(h)), true
	default:
		return RuntimeMiddleware{}, false
	}
}

func sameInstance(a, b any) bool {
	if a == nil || b == nil {
		return false
	}
	ta := reflect.TypeOf(a)
	tb := reflect.TypeOf(b)
	if ta != tb || !ta.Comparable() {
		return false
	}
	return a == b
}
