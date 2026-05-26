package producer

import (
	"github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/runtime"
)

const (
	Name = "queue.producer"
)

// Producer is the runtime-facing queue producer capability.
// It only exposes enqueue APIs and does not start queue workers.
type Producer = queue.Client
type Client = queue.Client
type Config = queue.Config

type ConfigLoader func(app *runtime.App) (Config, error)
