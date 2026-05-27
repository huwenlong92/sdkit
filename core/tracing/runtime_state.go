package tracing

import (
	"context"

	pkgtracing "github.com/huwenlong92/sdkit/pkg/tracing"
)

var ErrNotCompiled = pkgtracing.ErrNotCompiled

func Enabled() bool {
	return pkgtracing.Enabled()
}

func Shutdown(ctx context.Context) error {
	return pkgtracing.Shutdown(ctx)
}

func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	return pkgtracing.Init(ctx, cfg)
}
