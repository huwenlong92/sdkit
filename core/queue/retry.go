package queue

import (
	"context"
	"errors"
	"time"
)

func DefaultRetryDelay(retryCount int, err error, _ *Message) time.Duration {
	if IsRateLimitError(err) {
		return 0
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return 30 * time.Second
	}
	return 2 * time.Minute
}
