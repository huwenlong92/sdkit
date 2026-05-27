package eventbus

import (
	"context"
	"fmt"

	"github.com/huwenlong92/sdkit/core/tracing"
)

const eventbusTracerName = "sdkitgo/core/eventbus"

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

func recordHandlerSpanError(span tracing.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(tracing.StatusError, err.Error())
}
