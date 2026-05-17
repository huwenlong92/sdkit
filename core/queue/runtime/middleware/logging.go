package middleware

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/queue"

	"go.uber.org/zap"
)

func Logging(loggers ...*zap.Logger) queue.Middleware {
	log := logger.L
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return func(next queue.HandlerFunc) queue.HandlerFunc {
		return func(ctx context.Context, msg *queue.Message) error {
			startedAt := time.Now()
			taskLog := queueLogger(ctx, log)
			taskLog.Info("队列任务开始", messageFields(msg)...)

			err := next(ctx, msg)
			fields := append(messageFields(msg), zap.Duration("duration", time.Since(startedAt)))
			if err != nil {
				taskLog.Error("队列任务失败", append(fields, zap.Error(err))...)
				return err
			}
			taskLog.Info("队列任务完成", fields...)
			return nil
		}
	}
}

func messageFields(msg *queue.Message) []zap.Field {
	if msg == nil {
		return nil
	}
	fields := make([]zap.Field, 0, 5)
	if msg.ID != "" {
		fields = append(fields, zap.String("task_id", msg.ID))
	}
	if msg.Queue != "" {
		fields = append(fields, zap.String("queue", msg.Queue))
	}
	if msg.Type != "" {
		fields = append(fields, zap.String("type", msg.Type))
	}
	fields = append(fields,
		zap.Int("retry_count", msg.RetryCount),
		zap.Int("max_retry", msg.MaxRetry),
	)
	return fields
}

func queueLogger(ctx context.Context, log *zap.Logger) *zap.Logger {
	if log == nil {
		log = zap.NewNop()
	}
	fields := logger.ContextFields(ctx)
	if !hasField(fields, logger.TraceIDKey) {
		fields = append(fields, zap.String(logger.TraceIDKey, ""))
	}
	return log.With(fields...)
}

func hasField(fields []zap.Field, key string) bool {
	for _, field := range fields {
		if field.Key == key {
			return true
		}
	}
	return false
}
