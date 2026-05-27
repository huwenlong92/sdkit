//go:build !sdkit_tracing

package tracing

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
)

var ErrNotCompiled = errors.New("tracing enabled but binary was built without sdkit_tracing")

func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	cfg = normalizeConfig(cfg)
	otel.SetTextMapPropagator(NewPropagator())
	enabled.Store(false)
	setShutdown(noopShutdown)
	if cfg.Enabled {
		return nil, ErrNotCompiled
	}
	return noopShutdown, nil
}
