package eventbus

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/tracing"

	"go.uber.org/zap"
)

const eventbusTracerName = "sdkitgo/core/eventbus"

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

func Tracing() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, event *Event) (err error) {
			ctx = ContextWithEvent(ctx, event)
			ctx, span := startHandlerSpan(ctx, event)
			defer func() {
				if recovered := recover(); recovered != nil {
					recordHandlerSpanError(span, fmt.Errorf("panic: %v", recovered))
					span.End()
					panic(recovered)
				}
				recordHandlerSpanError(span, err)
				span.End()
			}()
			return next(ctx, event)
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

func startHandlerSpan(ctx context.Context, event *Event) (context.Context, tracing.Span) {
	attrs := []tracing.Attr{
		tracing.String("messaging.system", "eventbus"),
		tracing.String("messaging.destination.name", eventTopic(event)),
		tracing.String("messaging.operation.name", "process"),
	}
	if eventID(event) != "" {
		attrs = append(attrs, tracing.String("eventbus.event.id", eventID(event)))
	}
	name := "eventbus.handle"
	if eventTopic(event) != "" {
		name += " " + eventTopic(event)
	}
	ctx, span := tracing.StartSpanWithOptions(ctx, name, tracing.SpanOptions{
		TracerName: eventbusTracerName,
		Kind:       tracing.SpanKindConsumer,
	}, attrs...)
	tracing.SetSpanCorrelationAttributes(ctx, span)
	return ctx, span
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

func recordHandlerSpanError(span tracing.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(tracing.StatusError, err.Error())
}
