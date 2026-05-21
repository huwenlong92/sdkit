package ratelimit

import (
	"context"

	rlMiddleware "github.com/huwenlong92/sdkit/core/ratelimit/middleware"
	redisfacade "github.com/huwenlong92/sdkit/core/redis/facade"
	"github.com/huwenlong92/sdkit/core/runtime"
	"github.com/huwenlong92/sdkit/pkg/ratelimit/store"

	goredis "github.com/redis/go-redis/v9"
)

type UseOption func(*useOptions)

type useOptions struct {
	config       Config
	hasConfig    bool
	configLoader ConfigLoader
	store        Store
	redisClient  *goredis.Client
	prefix       string
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

func WithStore(s Store) UseOption {
	return func(o *useOptions) {
		o.store = s
	}
}

func WithRedisClient(client *goredis.Client) UseOption {
	return func(o *useOptions) {
		o.redisClient = client
	}
}

func WithPrefix(prefix string) UseOption {
	return func(o *useOptions) {
		o.prefix = prefix
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
	var runtimeStore Store
	var ownStore bool

	dependencies := []runtime.Dependency{
		runtime.Optional("bootstrap"),
		runtime.Optional(string(redisfacade.KeyRedis)),
	}
	dependencies = append(dependencies, o.dependencies...)

	return runtime.NewCapabilityWithMetadataAndDependencies(runtime.CapabilityMetadata{
		Name:        string(KeyRateLimit),
		Description: "Rate limit shared store",
		Group:       runtime.GroupSystem,
		Scope:       runtime.ScopeGlobal,
		Internal:    o.internal,
	}, dependencies, func(app *runtime.App) error {
		if o.configLoader != nil {
			if _, err := o.configLoader(app); err != nil {
				return err
			}
		}

		rateStore := o.store
		if rateStore == nil {
			rdb := o.redisClient
			if rdb == nil {
				rdb = redisfacade.ClientFrom(app)
			}
			if rdb != nil {
				if o.prefix != "" {
					rateStore = store.NewRedisStoreWithPrefix(rdb, o.prefix)
				} else {
					rateStore = store.NewRedisStore(rdb)
				}
			} else {
				rateStore = store.NewMemoryStore()
			}
			ownStore = true
		}
		runtimeStore = rateStore
		rlMiddleware.SetStore(rateStore)
		return app.Container().Bind(KeyRateLimit, rateStore)
	}, func(context.Context) error {
		rlMiddleware.SetStore(nil)
		if ownStore && runtimeStore != nil {
			return runtimeStore.Close()
		}
		return nil
	})
}
