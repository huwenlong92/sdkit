package middleware

import (
	"context"
	"errors"

	"github.com/huwenlong92/sdkit/core/queue"
)

func DeadLetter(deadletter queue.DeadLetter) queue.Middleware {
	if deadletter == nil {
		return nil
	}
	return func(next queue.HandlerFunc) queue.HandlerFunc {
		return func(ctx context.Context, msg *queue.Message) error {
			if ctx == nil {
				ctx = context.Background()
			}
			err := next(ctx, msg)
			if err == nil || queue.IsIgnoredError(err) || !shouldDeadLetter(msg, err) {
				return err
			}
			if pushErr := deadletter.Push(ctx, msg, err); pushErr != nil {
				return errors.Join(err, pushErr)
			}
			queue.TransitionTaskState(msg, queue.TaskFailed)
			queue.TransitionTaskState(msg, queue.TaskDeadLetter)
			return queue.NewDeadLetterError(err)
		}
	}
}

func shouldDeadLetter(msg *queue.Message, err error) bool {
	if err == nil {
		return false
	}
	if queue.IsFatalError(err) {
		return true
	}
	if msg == nil {
		return false
	}
	return msg.MaxRetry >= 0 && msg.RetryCount >= msg.MaxRetry
}
