package middleware

import (
	"github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/tracing"
)

func Tracing() queue.Middleware {
	return queue.ContextChain(func(c *queue.HandlerContext) error {
		msg := c.Message
		name := "handler::task"
		attrs := []tracing.Attr{
			tracing.String("worker.component", "queue"),
		}
		if msg != nil {
			name = "handler::" + msg.Type
			attrs = append(attrs,
				tracing.String("messaging.destination.name", msg.Queue),
				tracing.String("messaging.message.id", msg.ID),
				tracing.String("messaging.message.type", msg.Type),
				tracing.Int("messaging.message.retry_count", msg.RetryCount),
				tracing.Int("messaging.message.max_retry", msg.MaxRetry),
			)
		}

		ctx, span := tracing.StartSpanWithOptions(c.Context(), name, tracing.SpanOptions{
			TracerName: "sdkitgo/core/queue",
			Kind:       tracing.SpanKindInternal,
		}, attrs...)
		queue.SetSpanCorrelationAttributes(ctx, span)
		c.SetContext(ctx)
		defer span.End()

		err := c.Next()
		if err != nil {
			span.RecordError(err)
			span.SetStatus(tracing.StatusError, err.Error())
		}
		return err
	})
}
