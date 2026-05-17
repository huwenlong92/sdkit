package middleware

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"
)

const (
	MetricQueueTaskTotal        = "queue_task_total"
	MetricQueueTaskSuccessTotal = "queue_task_success_total"
	MetricQueueTaskFailTotal    = "queue_task_fail_total"
	MetricQueueTaskDuration     = "queue_task_duration"
	MetricQueueTaskRetryTotal   = "queue_task_retry_total"
)

func Metrics(recorder queue.MetricsRecorder) queue.Middleware {
	if recorder == nil {
		return nil
	}
	return func(next queue.HandlerFunc) queue.HandlerFunc {
		return func(ctx context.Context, msg *queue.Message) error {
			if ctx == nil {
				ctx = context.Background()
			}
			startedAt := time.Now()
			recorder.IncCounter(ctx, MetricQueueTaskTotal, metricsLabels(msg, "started"), 1)
			if msg != nil && msg.RetryCount > 0 {
				recorder.IncCounter(ctx, MetricQueueTaskRetryTotal, metricsLabels(msg, "retry"), int64(msg.RetryCount))
			}

			err := next(ctx, msg)
			status := "success"
			if err != nil {
				status = "failure"
			}
			labels := metricsLabels(msg, status)
			recorder.ObserveDuration(ctx, MetricQueueTaskDuration, labels, time.Since(startedAt))
			if err != nil {
				recorder.IncCounter(ctx, MetricQueueTaskFailTotal, labels, 1)
				return err
			}
			recorder.IncCounter(ctx, MetricQueueTaskSuccessTotal, labels, 1)
			return nil
		}
	}
}

func metricsLabels(msg *queue.Message, status string) queue.MetricsLabels {
	labels := queue.MetricsLabels{Status: status}
	if msg == nil {
		return labels
	}
	labels.Queue = msg.Queue
	labels.TaskType = msg.Type
	if worker, ok := queue.MessageMetadataString(msg, queue.MessageMetadataWorker); ok {
		labels.Worker = worker
	}
	return labels
}
