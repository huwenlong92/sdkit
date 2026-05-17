package retry

import (
	"time"

	"github.com/huwenlong92/sdkit/core/queue"
)

func DefaultDelay(retryCount int, err error, msg *queue.Message) time.Duration {
	return queue.DefaultRetryDelay(retryCount, err, msg)
}
