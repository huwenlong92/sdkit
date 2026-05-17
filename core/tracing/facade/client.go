package tracing

import (
	"context"

	coretracing "github.com/huwenlong92/sdkit/core/tracing"
)

func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	return coretracing.Init(ctx, cfg)
}

func Shutdown(ctx context.Context) error {
	return coretracing.Shutdown(ctx)
}

func Enabled() bool {
	return coretracing.Enabled()
}
