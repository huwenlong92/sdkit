package queue

import "context"

type ContextHandler func(*HandlerContext) error

type HandlerContext struct {
	ctx      context.Context
	Message  *Message
	handlers []ContextHandler
	index    int
}

func newHandlerContext(ctx context.Context, msg *Message, handlers []ContextHandler) *HandlerContext {
	if ctx == nil {
		ctx = context.Background()
	}
	return &HandlerContext{
		ctx:      ctx,
		Message:  msg,
		handlers: handlers,
		index:    -1,
	}
}

func (c *HandlerContext) Context() context.Context {
	if c == nil || c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c *HandlerContext) SetContext(ctx context.Context) {
	if c == nil || ctx == nil {
		return
	}
	c.ctx = ctx
}

func (c *HandlerContext) Next() error {
	if c == nil {
		return nil
	}
	c.index++
	if c.index >= len(c.handlers) {
		return nil
	}
	return c.handlers[c.index](c)
}

func ContextChain(handlers ...ContextHandler) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, msg *Message) error {
			chain := make([]ContextHandler, 0, len(handlers)+1)
			for _, handler := range handlers {
				if handler != nil {
					chain = append(chain, handler)
				}
			}
			chain = append(chain, func(c *HandlerContext) error {
				return next(c.Context(), c.Message)
			})
			return newHandlerContext(ctx, msg, chain).Next()
		}
	}
}
