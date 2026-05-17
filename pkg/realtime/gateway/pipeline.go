package gateway

import "github.com/huwenlong92/sdkit/core/realtime"

func BuildPipeline(route *realtime.Route, middlewares ...realtime.MiddlewareFunc) realtime.HandlerFunc {
	if route == nil {
		return nil
	}
	if len(middlewares) == 0 && route.Compiled != nil {
		return route.Compiled
	}
	handler := route.Handler
	if handler == nil {
		handler = pipeline(route.Handlers...)
	}
	chain := append([]realtime.MiddlewareFunc(nil), route.Middleware...)
	chain = append(chain, middlewares...)
	return compileHandlers(handler, chain...)
}

func compileHandlers(handler realtime.HandlerFunc, middlewares ...realtime.MiddlewareFunc) realtime.HandlerFunc {
	if handler == nil {
		return nil
	}
	handlers := make([]realtime.HandlerFunc, 0, len(middlewares)+1)
	for _, middleware := range middlewares {
		if middleware == nil {
			continue
		}
		handlers = append(handlers, middleware(func(ctx *realtime.ActionContext) error {
			if ctx == nil {
				return realtime.ErrInvalidAction
			}
			return ctx.Next()
		}))
	}
	handlers = append(handlers, handler)
	return func(ctx *realtime.ActionContext) error {
		if ctx == nil {
			return realtime.ErrInvalidAction
		}
		return ctx.RunHandlers(handlers...)
	}
}

func pipeline(handlers ...realtime.HandlerFunc) realtime.HandlerFunc {
	return func(ctx *realtime.ActionContext) error {
		for _, handler := range handlers {
			if handler == nil {
				return realtime.ErrNilActionHandler
			}
			if ctx != nil && ctx.IsAborted() {
				return nil
			}
			if err := handler(ctx); err != nil {
				return err
			}
		}
		return nil
	}
}
