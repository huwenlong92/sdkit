package producer

import (
	corequeue "github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/runtime"
)

const (
	Name = "queue.producer"
)

// Producer is the runtime-facing queue producer capability.
// It only exposes enqueue APIs and does not start queue workers.
type Producer = corequeue.Client
type Client = corequeue.Client
type Config = corequeue.Config

type ConfigLoader func(app *runtime.App) (Config, error)
