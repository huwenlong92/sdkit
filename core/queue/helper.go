package queue

import (
	"context"
	"time"
)

func Push(ctx context.Context, taskType string, payload any, opts ...Option) (*TaskInfo, error) {
	return Enqueue(ctx, NewTask(taskType, payload), opts...)
}

func Delay(ctx context.Context, taskType string, payload any, delay time.Duration, opts ...Option) (*TaskInfo, error) {
	opts = append([]Option{WithDelay(delay)}, opts...)
	return Push(ctx, taskType, payload, opts...)
}
