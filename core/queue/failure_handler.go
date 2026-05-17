package queue

import (
	"context"

	"github.com/huwenlong92/sdkit/core/logger"

	"go.uber.org/zap"
)

func LogFailureHandler(message string) FailureHandler {
	if message == "" {
		message = "队列任务失败"
	}
	return func(ctx context.Context, failure *Failure) {
		if failure == nil {
			return
		}
		fields := []zap.Field{
			zap.String("task_id", failure.TaskID),
			zap.String("queue", failure.Queue),
			zap.String("type", failure.Type),
			zap.Int("retry_count", failure.RetryCount),
			zap.Int("max_retry", failure.MaxRetry),
			zap.Bool("rate_limited", failure.RateLimited),
			zap.Error(failure.Err),
		}
		fields = appendFailureField(fields, logger.TraceIDKey, TraceIDFromHeaders(failure.Headers))
		fields = appendFailureField(fields, "track_id", TrackIDFromHeaders(failure.Headers))
		fields = appendFailureField(fields, "request_id", RequestIDFromHeaders(failure.Headers))
		fields = appendFailureField(fields, "span_id", SpanIDFromHeaders(failure.Headers))
		logger.WithContext(ctx, logger.L).Error(message, fields...)
	}
}

func appendFailureField(fields []zap.Field, key string, value string) []zap.Field {
	if value == "" {
		return fields
	}
	return append(fields, zap.String(key, value))
}
