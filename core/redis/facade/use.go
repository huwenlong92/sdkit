package redis

import (
	"context"
	"errors"

	corelogger "github.com/huwenlong92/sdkit/core/logger"
	loggerfacade "github.com/huwenlong92/sdkit/core/logger/facade"
	coreredis "github.com/huwenlong92/sdkit/core/redis"
	"github.com/huwenlong92/sdkit/core/runtime"

	"go.uber.org/zap"
)

var ErrConfigRequired = errors.New("redis facade: config or client required")

type UseOption func(*useOptions)

type useOptions struct {
	config       Config
	hasConfig    bool
	configLoader ConfigLoader
	client       *RuntimeClient
	logger       *zap.Logger
	dependencies []runtime.Dependency
	internal     bool
}

func defaultUseOptions() useOptions {
	return useOptions{
		dependencies: []runtime.Dependency{
			runtime.OptionalBootstrap(),
			runtime.Optional(loggerfacade.Name),
		},
		internal: true,
	}
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

func WithClient(client *RuntimeClient) UseOption {
	return func(o *useOptions) {
		o.client = client
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

func WithExternal() UseOption {
	return func(o *useOptions) {
		o.internal = false
	}
}

func Use(opts ...UseOption) runtime.Capability {
	o := defaultUseOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	return runtime.NewCapabilityWithMetadataAndDependencies(runtime.CapabilityMetadata{
		Name:        Name,
		Description: "Redis client",
		Group:       runtime.GroupSystem,
		Scope:       runtime.ScopeGlobal,
		Internal:    o.internal,
	}, o.dependencies, func(app *runtime.App) error {
		log := o.logger
		if log == nil {
			log = corelogger.From(app)
		}

		client := o.client
		config := o.config
		if client == nil {
			hasConfig := o.hasConfig
			if o.configLoader != nil {
				loaded, err := o.configLoader(app)
				if err != nil {
					return err
				}
				config = loaded
				hasConfig = true
			}
			if !hasConfig {
				return ErrConfigRequired
			}
			client = coreredis.New(config, log)
			if err := client.Ping(app.Context()); err != nil {
				_ = client.Close()
				return err
			}
		}
		if err := coreredis.Bind(app, client); err != nil {
			return err
		}
		log.Info("Redis初始化成功",
			zap.String(corelogger.TraceIDKey, ""),
			zap.String("addr", config.Addr),
		)
		return nil
	}, func(context.Context) error {
		return coreredis.Close()
	})
}
