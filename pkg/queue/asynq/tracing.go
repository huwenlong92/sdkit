//go:build sdkit_queue_asynq

package asynq

import (
	"context"
	"fmt"

	"github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/tracing"
)

func startWorkerSpan(ctx context.Context, msg *queue.Message) (context.Context, tracing.Span) {
	attrs := []tracing.Attr{
		tracing.String("messaging.system", "asynq"),
		tracing.String("messaging.operation", "process"),
	}
	spanName := "consumer::task"
	if msg != nil {
		spanName = "consumer::" + msg.Type
		attrs = append(attrs,
			tracing.String("messaging.destination.name", msg.Queue),
			tracing.String("messaging.message.id", msg.ID),
			tracing.String("messaging.message.type", msg.Type),
			tracing.Int("messaging.message.retry_count", msg.RetryCount),
			tracing.Int("messaging.message.max_retry", msg.MaxRetry),
		)
	}
	ctx, span := tracing.StartSpanWithOptions(ctx, spanName, tracing.SpanOptions{
		TracerName: "sdkitgo/core/queue",
		Kind:       tracing.SpanKindConsumer,
	}, attrs...)
	queue.SetSpanCorrelationAttributes(ctx, span)
	return ctx, span
}

func startEnqueueSpan(ctx context.Context, taskType string, opts queue.EnqueueOptions) (context.Context, tracing.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	attrs := []tracing.Attr{
		tracing.String("messaging.system", "asynq"),
		tracing.String("messaging.operation", "publish"),
		tracing.String("messaging.destination.name", opts.Queue),
		tracing.String("messaging.message.id", opts.TaskID),
		tracing.String("messaging.message.type", taskType),
	}
	ctx, span := tracing.StartSpanWithOptions(ctx, "producer::"+taskType, tracing.SpanOptions{
		TracerName: "sdkitgo/core/queue",
		Kind:       tracing.SpanKindProducer,
	}, attrs...)
	queue.SetSpanCorrelationAttributes(ctx, span)
	return ctx, span
}

func setEnqueueResultAttributes(span tracing.Span, info *queue.TaskInfo) {
	if span == nil || info == nil {
		return
	}
	span.SetAttributes(
		tracing.String("messaging.message.id", info.ID),
		tracing.String("messaging.destination.name", info.Queue),
	)
}

func recordQueueSpanError(span tracing.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(tracing.StatusError, err.Error())
}

func recordQueueSpanPanic(span tracing.Span, recovered interface{}) {
	if span == nil {
		return
	}
	span.RecordError(fmt.Errorf("panic: %v", recovered))
	span.SetStatus(tracing.StatusError, "panic")
}
