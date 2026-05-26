package sms

import (
	"context"
	"errors"

	"github.com/huwenlong92/sdkit/core/runtime"
	coresms "github.com/huwenlong92/sdkit/core/sms"
	_ "github.com/huwenlong92/sdkit/pkg/sms/driver/aliyun"
	_ "github.com/huwenlong92/sdkit/pkg/sms/driver/feige"
)

type ConfigLoader func(app *runtime.App) (Config, error)

type UseOption func(*useOptions)

type useOptions struct {
	config       Config
	hasConfig    bool
	configLoader ConfigLoader
	manager      *Manager
	middleware   []Middleware
	dependencies []runtime.Dependency
	internal     bool
	optional     bool
}

func defaultUseOptions() useOptions {
	return useOptions{
		dependencies: []runtime.Dependency{
			runtime.OptionalBootstrap(),
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

func WithManager(manager *Manager) UseOption {
	return func(o *useOptions) {
		o.manager = manager
	}
}

func WithMiddleware(middleware ...Middleware) UseOption {
	return func(o *useOptions) {
		o.middleware = append(o.middleware, middleware...)
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

func WithOptional() UseOption {
	return func(o *useOptions) {
		o.optional = true
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
		Description: "SMS sender",
		Group:       runtime.GroupSystem,
		Scope:       runtime.ScopeGlobal,
		Internal:    o.internal,
	}, o.dependencies, func(app *runtime.App) error {
		manager := o.manager
		if manager == nil {
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
			if !hasConfig {
				if o.optional {
					return nil
				}
				return coresms.ErrNotConfigured
			}
			var err error
			manager, err = coresms.NewManager(cfg, o.middleware...)
			if err != nil {
				if o.optional && errors.Is(err, coresms.ErrNotConfigured) {
					return nil
				}
				return err
			}
		}
		return coresms.Bind(app, manager)
	}, func(context.Context) error {
		return coresms.Close()
	})
}

func RateLimitMiddleware(limiter RateLimiter, rule RateLimitRule) Middleware {
	return coresms.RateLimitMiddleware(limiter, rule)
}

func PhoneKey(req coresms.Request) string {
	return coresms.PhoneKey(req)
}
