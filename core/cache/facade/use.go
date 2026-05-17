package cache

import (
	"context"

	corecache "github.com/huwenlong92/sdkit/core/cache"
	coreconfig "github.com/huwenlong92/sdkit/core/config"
	redisfacade "github.com/huwenlong92/sdkit/core/redis/facade"
	"github.com/huwenlong92/sdkit/core/runtime"
)

type UseOption func(*useOptions)

type useOptions struct {
	config       Config
	hasConfig    bool
	configLoader ConfigLoader
	cache        Cache
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

func WithCache(c Cache) UseOption {
	return func(o *useOptions) {
		o.cache = c
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
		runtime.Optional(string(redisfacade.KeyRedis)),
	}
	dependencies = append(dependencies, o.dependencies...)

	return runtime.NewCapabilityWithMetadataAndDependencies(runtime.CapabilityMetadata{
		Name:        string(KeyCache),
		Description: "Cache store",
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

		cache := o.cache
		if cache == nil {
			if !hasConfig && coreconfig.V != nil {
				if err := coreconfig.V.UnmarshalKey("cache", &config); err != nil {
					return err
				}
			}
			prefix := "cache:"
			if config.Prefix != "" {
				prefix = config.Prefix
			}
			if client := redisfacade.From(app); client != nil && client.Rdb != nil {
				cache = corecache.New(corecache.WithRedis(client.Rdb), corecache.WithPrefix(prefix))
			} else {
				cache = corecache.New()
			}
		}
		return corecache.Bind(app, cache)
	}, func(context.Context) error {
		corecache.Close()
		return nil
	})
}
