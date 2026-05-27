package eventbus

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/huwenlong92/sdkit/core/logger"

	"go.uber.org/zap"
)

type Middleware func(Handler) Handler

func Chain(handler Handler, middlewares ...Middleware) Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		if middlewares[i] != nil {
			handler = middlewares[i](handler)
		}
	}
	return handler
}

func Recover(log *zap.Logger) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, event *Event) (err error) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.WithContext(ctx, log).Warn(
						"eventbus handler panic recovered",
						zap.Any("err", recovered),
						zap.String("topic", eventTopic(event)),
						zap.String("event_id", eventID(event)),
						zap.ByteString("stack", debug.Stack()),
					)
					err = nil
				}
			}()
			return next(ctx, event)
		}
	}
}

func Logger(log *zap.Logger) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, event *Event) error {
			err := next(ctx, event)
			if err != nil {
				logger.WithContext(ctx, log).Warn(
					"eventbus handler error",
					zap.Error(err),
					zap.String("topic", eventTopic(event)),
					zap.String("event_id", eventID(event)),
				)
			}
			return err
		}
	}
}

func SafeHandle(ctx context.Context, event *Event, handler Handler, log *zap.Logger) (err error) {
	if handler == nil {
		return ErrNilHandler
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.WithContext(ctx, log).Warn(
				"eventbus handler panic recovered",
				zap.Any("err", recovered),
				zap.String("topic", eventTopic(event)),
				zap.String("event_id", eventID(event)),
				zap.ByteString("stack", debug.Stack()),
			)
			err = nil
		}
	}()
	err = handler(ContextWithEvent(ctx, event), event)
	if err != nil {
		return fmt.Errorf("eventbus handler %s: %w", eventTopic(event), err)
	}
	return nil
}

func eventID(event *Event) string {
	if event == nil {
		return ""
	}
	return event.ID
}

func eventTopic(event *Event) string {
	if event == nil {
		return ""
	}
	return event.Topic
}
