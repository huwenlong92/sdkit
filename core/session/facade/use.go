package session

import (
	"context"

	redisfacade "github.com/huwenlong92/sdkit/core/redis/facade"
	"github.com/huwenlong92/sdkit/core/runtime"
	coresession "github.com/huwenlong92/sdkit/core/session"

	goredis "github.com/redis/go-redis/v9"
)

type UseOption func(*useOptions)

type useOptions struct {
	name         runtime.Key
	config       Config
	hasConfig    bool
	configLoader ConfigLoader
	redisClient  *goredis.Client
	dependencies []runtime.Dependency
	internal     bool
}

func WithName(name string) UseOption {
	return func(o *useOptions) {
		o.name = runtime.Key(name)
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

func WithRedisClient(client *goredis.Client) UseOption {
	return func(o *useOptions) {
		o.redisClient = client
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
	o := useOptions{name: KeySession}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if o.name == "" {
		o.name = KeySession
	}

	dependencies := []runtime.Dependency{
		runtime.Optional("bootstrap"),
		runtime.Optional(string(redisfacade.KeyRedis)),
	}
	dependencies = append(dependencies, o.dependencies...)

	return runtime.NewCapabilityWithMetadataAndDependencies(runtime.CapabilityMetadata{
		Name:        string(o.name),
		Description: "Session store",
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

		rdb := o.redisClient
		if rdb == nil {
			rdb = redisfacade.ClientFrom(app)
		}
		if o.name == KeySession && hasConfig {
			coresession.Init(rdb, &config)
			return app.Container().Bind(KeySession, coresession.GetStore())
		}
		if o.name == KeySession {
			coresession.Init(rdb, nil)
			return app.Container().Bind(KeySession, coresession.GetStore())
		}
		var store coresession.Store
		if hasConfig {
			store = coresession.NewStore(rdb, &config)
		} else {
			store = coresession.NewStore(rdb, nil)
		}
		return app.Container().Bind(o.name, store)
	}, func(context.Context) error {
		return nil
	})
}
