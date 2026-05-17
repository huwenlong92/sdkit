package middleware

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/queue"

	"go.uber.org/zap"
)

func Recover(loggers ...*zap.Logger) queue.Middleware {
	log := logger.L
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return func(next queue.HandlerFunc) queue.HandlerFunc {
		return func(ctx context.Context, msg *queue.Message) (err error) {
			defer func() {
				if recovered := recover(); recovered != nil {
					err = fmt.Errorf("queue handler panic: %v", recovered)
					fields := append(messageFields(msg),
						zap.Error(err),
						zap.ByteString("stack", debug.Stack()),
					)
					queueLogger(ctx, log).Error("队列任务 panic", fields...)
				}
			}()
			return next(ctx, msg)
		}
	}
}
