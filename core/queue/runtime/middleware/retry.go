package middleware

import (
	"context"

	"github.com/huwenlong92/sdkit/core/queue"
)

func Retry(strategy queue.RetryStrategy) queue.Middleware {
	if strategy == nil {
		return nil
	}
	return func(next queue.HandlerFunc) queue.HandlerFunc {
		return func(ctx context.Context, msg *queue.Message) error {
			if ctx == nil {
				ctx = context.Background()
			}
			err := next(ctx, msg)
			if err == nil ||
				queue.IsRateLimitError(err) ||
				queue.IsDeadLetterError(err) ||
				queue.IsFatalError(err) ||
				queue.IsIgnoredError(err) ||
				(msg != nil && msg.State == queue.TaskDeadLetter) {
				return err
			}
			if runtimeErr, ok := queue.RuntimeErrorFrom(err); ok && runtimeErr.Retryable && runtimeErr.RetryIn > 0 {
				return err
			}
			retryCount := 0
			if msg != nil {
				retryCount = msg.RetryCount
			}
			retryIn, ok := strategy.NextRetry(ctx, msg, retryCount, err)
			if !ok {
				return err
			}
			queue.TransitionTaskState(msg, queue.TaskRetrying)
			return queue.RetryableAfter(retryIn, err)
		}
	}
}
