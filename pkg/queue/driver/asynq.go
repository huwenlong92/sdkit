//go:build sdkit_queue_asynq

package driver

import "github.com/huwenlong92/sdkit/pkg/queue/asynq"

func init() {
	register("asynq", asynq.Register)
}
