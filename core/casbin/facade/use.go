package casbin

import (
	corecasbin "github.com/huwenlong92/sdkit/core/casbin"
	databasefacade "github.com/huwenlong92/sdkit/core/database/facade"
	corelogger "github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/runtime"
)

type UseOption func(*useOptions)

type useOptions struct {
	config         Config
	configLoader   ConfigLoader
	database       *Database
	databaseLoader DatabaseLoader
	manager        *Manager
	dependencies   []runtime.Dependency
	internal       bool
}

func defaultUseOptions() useOptions {
	return useOptions{
		dependencies: []runtime.Dependency{
			runtime.Optional("bootstrap"),
			runtime.Optional(string(corelogger.KeyLogger)),
			runtime.Optional(string(databasefacade.KeyDatabase)),
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

func WithDatabase(db *Database) UseOption {
	return func(o *useOptions) {
		o.database = db
	}
}

func WithDatabaseLoader(loader DatabaseLoader) UseOption {
	return func(o *useOptions) {
		o.databaseLoader = loader
	}
}

func WithCapabilityManager(manager *Manager) UseOption {
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
		Name:        string(KeyCasbin),
		Description: "Casbin RBAC manager",
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

		db := o.database
		if o.databaseLoader != nil {
			loaded, err := o.databaseLoader(app)
			if err != nil {
				return err
			}
			db = loaded
		}
		if db == nil {
			db = databasefacade.From(app)
		}

		manager := o.manager
		if manager == nil {
			created, err := corecasbin.NewContext(app.Context(), db, config)
			if err != nil {
				return err
			}
			manager = created
		}
		return corecasbin.Bind(app, manager)
	}, nil)
}
