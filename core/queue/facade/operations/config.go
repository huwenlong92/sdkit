package operations

import (
	corequeue "github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/runtime"
)

const (
	Name = "queue.operations"
)

type Config struct {
	Queue    corequeue.Config
	Metadata corequeue.RuntimeMetadata
}

type ConfigLoader func(app *runtime.App) (Config, error)

func NewConfig(name string, service string, cfg corequeue.Config) Config {
	return Config{
		Queue:    cfg,
		Metadata: corequeue.RuntimeMetadataFromConfig(name, service, cfg),
	}
}
