package gateway

import (
	"context"

	"github.com/huwenlong92/sdkit/core/realtime"
	"github.com/huwenlong92/sdkit/pkg/realtime/transport"
)

type Runtime struct {
	router     *Router
	dispatcher realtime.Dispatcher
	publisher  realtime.Publisher
	lifecycle  transport.Lifecycle
}

type Option func(*Runtime)

func WithRouter(router *Router) Option {
	return func(r *Runtime) {
		if router != nil {
			r.router = router
		}
	}
}

func WithDispatcher(dispatcher realtime.Dispatcher) Option {
	return func(r *Runtime) {
		r.dispatcher = dispatcher
	}
}

func WithPublisher(publisher realtime.Publisher) Option {
	return func(r *Runtime) {
		r.publisher = publisher
	}
}

func WithLifecycle(lifecycle transport.Lifecycle) Option {
	return func(r *Runtime) {
		r.lifecycle = lifecycle
	}
}

func NewRuntime(opts ...Option) *Runtime {
	runtime := &Runtime{router: NewRouter()}
	for _, opt := range opts {
		if opt != nil {
			opt(runtime)
		}
	}
	return runtime
}

func (r *Runtime) Router() *Router {
	if r == nil {
		return nil
	}
	return r.router
}

func (r *Runtime) Handle(ctx *realtime.ActionContext) error {
	if r == nil || r.router == nil {
		return realtime.ErrNilRouter
	}
	if ctx == nil || ctx.Event == nil {
		return realtime.ErrInvalidAction
	}
	ctx.Event.Normalize()
	route, ok := r.router.Match(ctx.Event.Action)
	if !ok {
		return realtime.ErrActionNotFound
	}
	handler := route.Compiled
	if handler == nil {
		return realtime.ErrNilActionHandler
	}
	if ctx.Gateway == nil {
		ctx.Gateway = r
	}
	if r.lifecycle != nil && ctx.Client != nil {
		if err := r.lifecycle.OnActivity(ctx.Context(), ctx.Client); err != nil {
			return err
		}
	}
	return handler(ctx)
}

func (r *Runtime) Publish(ctx context.Context, evt *realtime.Event) error {
	if r == nil || r.publisher == nil {
		return realtime.ErrNilPublisher
	}
	return r.publisher.Broadcast(ctx, evt)
}

func (r *Runtime) DispatchEvent(ctx context.Context, evt *realtime.Event) error {
	if r == nil || r.dispatcher == nil {
		return realtime.ErrNilClient
	}
	return r.dispatcher.DispatchEvent(ctx, evt)
}

func (r *Runtime) DispatchLocal(ctx context.Context, evt *realtime.Event) error {
	if r == nil || r.dispatcher == nil {
		return realtime.ErrNilClient
	}
	return r.dispatcher.DispatchLocal(ctx, evt)
}

func (r *Runtime) DispatchClient(ctx context.Context, clientID string, evt *realtime.Event) error {
	if r == nil || r.dispatcher == nil {
		return realtime.ErrNilClient
	}
	return r.dispatcher.DispatchClient(ctx, clientID, evt)
}

func (r *Runtime) PushUser(ctx context.Context, userID string, evt *realtime.Event) error {
	if r == nil || r.dispatcher == nil {
		return realtime.ErrNilClient
	}
	return r.dispatcher.PushUser(ctx, userID, evt)
}

func (r *Runtime) PushClient(ctx context.Context, clientID string, evt *realtime.Event) error {
	if r == nil || r.dispatcher == nil {
		return realtime.ErrNilClient
	}
	return r.dispatcher.PushClient(ctx, clientID, evt)
}

func (r *Runtime) PushRoom(ctx context.Context, roomID string, evt *realtime.Event) error {
	if r == nil || r.dispatcher == nil {
		return realtime.ErrNilClient
	}
	return r.dispatcher.PushRoom(ctx, roomID, evt)
}

func (r *Runtime) Broadcast(ctx context.Context, evt *realtime.Event) error {
	if r == nil || r.dispatcher == nil {
		return realtime.ErrNilClient
	}
	return r.dispatcher.Broadcast(ctx, evt)
}

var _ realtime.Gateway = (*Runtime)(nil)
