package redis

import (
	"context"

	coreconfig "github.com/huwenlong92/sdkit/core/config"
	corelogger "github.com/huwenlong92/sdkit/core/logger"
	coreredis "github.com/huwenlong92/sdkit/core/redis"
	"github.com/huwenlong92/sdkit/core/runtime"

	"go.uber.org/zap"
)

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
		Name:        string(KeyRedis),
		Description: "Redis client",
		Group:       runtime.GroupSystem,
		Scope:       runtime.ScopeGlobal,
		Internal:    o.internal,
	}, dependencies, func(app *runtime.App) error {
		config := o.config
		hasConfig := o.hasConfig
		if o.configLoader != nil {
			loaded, err := o.configLoader(app)
			if err != nil {
				return err
			}
			config = loaded
			hasConfig = true
		}

		log := o.logger
		if log == nil {
			log = corelogger.From(app)
		}

		client := o.client
		if client == nil {
			if !hasConfig && coreconfig.V != nil {
				if err := coreconfig.V.UnmarshalKey("redis", &config); err != nil {
					return err
				}
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
