package operations

import (
	"github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/runtime"
)

const (
	Name = "queue.operations"
)

type Config struct {
	Queue    queue.Config
	Metadata queue.RuntimeMetadata
}

type ConfigLoader func(app *runtime.App) (Config, error)

func NewConfig(name string, service string, cfg queue.Config) Config {
	return Config{
		Queue:    cfg,
		Metadata: queue.RuntimeMetadataFromConfig(name, service, cfg),
	}
}
