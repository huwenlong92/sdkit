package storage

import (
	"context"

	coreconfig "github.com/huwenlong92/sdkit/core/config"
	"github.com/huwenlong92/sdkit/core/runtime"
	corestorage "github.com/huwenlong92/sdkit/core/storage"
)

type ConfigLoader func(app *runtime.App) (Config, error)

type UseOption func(*useOptions)

type useOptions struct {
	config       Config
	hasConfig    bool
	configLoader ConfigLoader
	manager      *Manager
	dependencies []runtime.Dependency
	internal     bool
}

func defaultUseOptions() useOptions {
	return useOptions{
		configLoader: loadConfigFromCore,
		dependencies: []runtime.Dependency{
			runtime.OptionalBootstrap(),
		},
		internal: true,
	}
}

func DefaultConfig() Config {
	return Config{
		Default: corestorage.DefaultStoreName,
		Stores: map[string]StoreConfig{
			corestorage.DefaultStoreName: {
				Driver:   "local",
				LocalDir: "storage",
			},
		},
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
	o := defaultUseOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	return runtime.NewCapabilityWithMetadataAndDependencies(runtime.CapabilityMetadata{
		Name:        Name,
		Description: "Storage manager",
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
				return corestorage.ErrNotConfigured
			}
			var err error
			manager, err = corestorage.NewManager(cfg)
			if err != nil {
				return err
			}
		}
		return corestorage.Bind(app, manager)
	}, func(context.Context) error {
		return corestorage.Close()
	})
}

func loadConfigFromCore(*runtime.App) (Config, error) {
	if coreconfig.V == nil {
		return DefaultConfig(), nil
	}
	if coreconfig.V.IsSet("storage") {
		var cfg Config
		if err := coreconfig.V.UnmarshalKey("storage", &cfg); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}
	return DefaultConfig(), nil
}
