package tracing

import (
	"context"

	"github.com/huwenlong92/sdkit/core/runtime"
	coretracing "github.com/huwenlong92/sdkit/core/tracing"
)

const Name = "tracing"

type Config = coretracing.Config

type ConfigLoader func(app *runtime.App) (Config, error)

func DefaultConfig() Config {
	return coretracing.DefaultConfig()
}

func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	return coretracing.Init(ctx, cfg)
}

func Shutdown(ctx context.Context) error {
	return coretracing.Shutdown(ctx)
}

func Enabled() bool {
	return coretracing.Enabled()
}
