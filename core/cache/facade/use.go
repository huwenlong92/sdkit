package cache

import (
	"context"

	corecache "github.com/huwenlong92/sdkit/core/cache"
	redisfacade "github.com/huwenlong92/sdkit/core/redis/facade"
	"github.com/huwenlong92/sdkit/core/runtime"

	"github.com/redis/go-redis/v9"
)

type UseOption func(*useOptions)

type useOptions struct {
	config       Config
	configLoader ConfigLoader
	cache        Cache
	dependencies []runtime.Dependency
	internal     bool
}

func defaultUseOptions() useOptions {
	return useOptions{
		dependencies: []runtime.Dependency{
			runtime.OptionalBootstrap(),
			runtime.Optional(redisfacade.Name),
		},
		internal: true,
	}
}

func WithConfig(cfg Config) UseOption {
	return func(o *useOptions) {
		o.config = cfg
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
		Description: "Cache store",
		Group:       runtime.GroupSystem,
		Scope:       runtime.ScopeGlobal,
		Internal:    o.internal,
	}, o.dependencies, func(app *runtime.App) error {
		config := o.config
		if o.configLoader != nil {
			loaded, err := o.configLoader(app)
			if err != nil {
				return err
			}
			config = loaded
		}

		cache := o.cache
		if cache == nil {
			var rdb *redis.Client
			if client := redisfacade.From(app); client != nil {
				rdb = client.Rdb
			}
			cache = corecache.NewFromConfig(&config, rdb)
		}
		return corecache.Bind(app, cache)
	}, func(context.Context) error {
		corecache.Close()
		return nil
	})
}
