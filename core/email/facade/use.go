package email

import (
	"context"
	"errors"

	coreemail "github.com/huwenlong92/sdkit/core/email"
	"github.com/huwenlong92/sdkit/core/runtime"
	_ "github.com/huwenlong92/sdkit/pkg/email/driver/smtp"
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
		Description: "Email sender",
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
				return coreemail.ErrNotConfigured
			}
			var err error
			manager, err = coreemail.NewManager(cfg, o.middleware...)
			if err != nil {
				if o.optional && errors.Is(err, coreemail.ErrNotConfigured) {
					return nil
				}
				return err
			}
		}
		return coreemail.Bind(app, manager)
	}, func(context.Context) error {
		return coreemail.Close()
	})
}
