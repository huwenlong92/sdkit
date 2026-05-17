package tracing

import (
	"context"

	corelogger "github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/runtime"
	coretracing "github.com/huwenlong92/sdkit/core/tracing"

	"go.uber.org/zap"
)

type UseOption func(*useOptions)

type useOptions struct {
	config       Config
	hasConfig    bool
	configLoader ConfigLoader
	serviceName  string
	environment  string
	logger       *zap.Logger
	dependencies []runtime.Dependency
	internal     bool
}

func WithConfig(cfg Config) UseOption {
	return func(o *useOptions) {
		o.config = cfg
		o.hasConfig = true
	}
}

func WithConfigLoader(loader ConfigLoader) UseOption {
	return func(o *useOptions) {
		o.configLoader = loader
	}
}

func WithServiceName(serviceName string) UseOption {
	return func(o *useOptions) {
		o.serviceName = serviceName
	}
}

func WithEnvironment(environment string) UseOption {
	return func(o *useOptions) {
		o.environment = environment
	}
}

func WithLogger(log *zap.Logger) UseOption {
	return func(o *useOptions) {
		o.logger = log
	}
}

func WithDependencies(deps ...runtime.Dependency) UseOption {
	return func(o *useOptions) {
		o.dependencies = append(o.dependencies, deps...)
	}
}

func WithInternal() UseOption {
	return func(o *useOptions) {
		o.internal = true
	}
}

func Use(opts ...UseOption) runtime.Capability {
	o := useOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	dependencies := []runtime.Dependency{
		runtime.Optional("bootstrap"),
		runtime.Optional(string(corelogger.KeyLogger)),
	}
	dependencies = append(dependencies, o.dependencies...)

	return runtime.NewCapabilityWithMetadataAndDependencies(runtime.CapabilityMetadata{
		Name:        Name,
		Description: "OpenTelemetry tracing",
		Group:       runtime.GroupSystem,
		Scope:       runtime.ScopeGlobal,
		Internal:    o.internal,
	}, dependencies, func(app *runtime.App) error {
		cfg := o.config
		if o.configLoader != nil {
			loaded, err := o.configLoader(app)
			if err != nil {
				return err
			}
			cfg = loaded
		}
		if o.serviceName != "" && cfg.ServiceName == "" {
			cfg.ServiceName = o.serviceName
		}
		if o.environment != "" && cfg.Environment == "" {
			cfg.Environment = o.environment
		}

		log := o.logger
		if log == nil {
			log = corelogger.From(app)
		}
		if _, err := coretracing.Init(app.Context(), cfg); err != nil {
			log.Error("Tracing初始化失败", zap.String(corelogger.TraceIDKey, ""), zap.Error(err))
			return err
		}
		if cfg.Enabled {
			log.Info("Tracing初始化成功",
				zap.String(corelogger.TraceIDKey, ""),
				zap.String("service_name", cfg.ServiceName),
				zap.String("endpoint", cfg.Endpoint),
			)
		}
		return nil
	}, func(ctx context.Context) error {
		return coretracing.Shutdown(ctx)
	})
}
