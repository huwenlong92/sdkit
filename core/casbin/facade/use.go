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
	hasConfig      bool
	configLoader   ConfigLoader
	database       *Database
	databaseLoader DatabaseLoader
	manager        *Manager
	dependencies   []runtime.Dependency
	internal       bool
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
		runtime.Optional(string(databasefacade.KeyDatabase)),
	}
	dependencies = append(dependencies, o.dependencies...)

	return runtime.NewCapabilityWithMetadataAndDependencies(runtime.CapabilityMetadata{
		Name:        string(KeyCasbin),
		Description: "Casbin RBAC manager",
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
			if !hasConfig {
				config = Config{}
			}
			created, err := corecasbin.NewContext(app.Context(), db, config)
			if err != nil {
				return err
			}
			manager = created
		}
		return corecasbin.Bind(app, manager)
	}, nil)
}
