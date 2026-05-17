package logger

import (
	"context"

	corelogger "github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/runtime"

	"go.uber.org/zap"
)

type UseOption func(*useOptions)

type useOptions struct {
	config       Config
	hasConfig    bool
	configLoader ConfigLoader
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

func WithLogger(log *zap.Logger) UseOption {
	return func(o *useOptions) {
		o.logger = log
	}
}

func WithConfigLoader(loader ConfigLoader) UseOption {
	return func(o *useOptions) {
		o.configLoader = loader
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

	dependencies := []runtime.Dependency{runtime.Optional("bootstrap")}
	dependencies = append(dependencies, o.dependencies...)

	return runtime.NewCapabilityWithMetadataAndDependencies(runtime.CapabilityMetadata{
		Name:        string(KeyLogger),
		Description: "Zap logger",
		Group:       runtime.GroupSystem,
		Scope:       runtime.ScopeGlobal,
		Internal:    o.internal,
	}, dependencies, func(app *runtime.App) error {
		log := o.logger
		if log == nil {
			cfg := o.config
			hasConfig := o.hasConfig
			if o.configLoader != nil {
				loaded, err := o.configLoader(app)
				if err != nil {
					return err
				}
				cfg = loaded
				hasConfig = true
			}
			if !hasConfig && corelogger.L != nil {
				log = corelogger.L
			} else {
				if err := corelogger.Configure(cfg); err != nil {
					return err
				}
				log = corelogger.L
			}
		} else {
			corelogger.L = log
		}
		return app.Container().Bind(KeyLogger, log)
	}, func(context.Context) error {
		corelogger.Sync()
		return nil
	})
}
