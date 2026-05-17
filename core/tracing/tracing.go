package tracing

import (
	"context"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var (
	shutdownMu     sync.Mutex
	globalShutdown = noopShutdown
	enabled        atomic.Bool
)

func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	cfg = normalizeConfig(cfg)
	otel.SetTextMapPropagator(NewPropagator())

	if !cfg.Enabled {
		otel.SetTracerProvider(otel.GetTracerProvider())
		enabled.Store(false)
		setShutdown(noopShutdown)
		return noopShutdown, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	exporterCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithTimeout(cfg.Timeout),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(exporterCtx, opts...)
	if err != nil {
		enabled.Store(false)
		if cfg.Strict {
			return nil, err
		}
		setShutdown(noopShutdown)
		return noopShutdown, nil
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			"",
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("deployment.environment", cfg.Environment),
		),
	)
	if err != nil {
		enabled.Store(false)
		if cfg.Strict {
			return nil, err
		}
		setShutdown(noopShutdown)
		return noopShutdown, nil
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	)
	otel.SetTracerProvider(provider)
	enabled.Store(true)
	setShutdown(provider.Shutdown)
	return provider.Shutdown, nil
}

func Enabled() bool {
	return enabled.Load()
}

func Shutdown(ctx context.Context) error {
	shutdownMu.Lock()
	shutdown := globalShutdown
	globalShutdown = noopShutdown
	shutdownMu.Unlock()
	enabled.Store(false)
	return shutdown(ctx)
}

func setShutdown(shutdown func(context.Context) error) {
	if shutdown == nil {
		shutdown = noopShutdown
	}
	shutdownMu.Lock()
	globalShutdown = shutdown
	shutdownMu.Unlock()
}

func noopShutdown(context.Context) error {
	return nil
}
