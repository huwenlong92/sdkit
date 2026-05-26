package database

import (
	"context"
	"errors"

	coredatabase "github.com/huwenlong92/sdkit/core/database"
	corelogger "github.com/huwenlong92/sdkit/core/logger"
	loggerfacade "github.com/huwenlong92/sdkit/core/logger/facade"
	"github.com/huwenlong92/sdkit/core/runtime"
	tracingfacade "github.com/huwenlong92/sdkit/core/tracing/facade"

	"go.uber.org/zap"
)

var ErrConfigRequired = errors.New("database facade: config or database required")

type UseOption func(*useOptions)

type useOptions struct {
	config       Config
	hasConfig    bool
	configLoader ConfigLoader
	mode         string
	modeLoader   ModeLoader
	database     *Database
	dependencies []runtime.Dependency
	internal     bool
}

func defaultUseOptions() useOptions {
	return useOptions{
		dependencies: []runtime.Dependency{
			runtime.OptionalBootstrap(),
			runtime.Optional(loggerfacade.Name),
			runtime.Optional(tracingfacade.Name),
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

func WithMode(mode string) UseOption {
	return func(o *useOptions) {
		o.mode = mode
	}
}

func WithModeLoader(loader ModeLoader) UseOption {
	return func(o *useOptions) {
		o.modeLoader = loader
	}
}

func WithDatabase(db *Database) UseOption {
	return func(o *useOptions) {
		o.database = db
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
		Description: "GORM and pgx database",
		Group:       runtime.GroupSystem,
		Scope:       runtime.ScopeGlobal,
		Internal:    o.internal,
	}, o.dependencies, func(app *runtime.App) error {
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

		mode := o.mode
		if o.modeLoader != nil {
			loaded, err := o.modeLoader(app)
			if err != nil {
				return err
			}
			mode = loaded
		}

		db := o.database
		if db == nil {
			if !hasConfig {
				return ErrConfigRequired
			}
			var err error
			db, err = coredatabase.New(app.Context(), config, mode)
			if err != nil {
				return err
			}
		}
		if err := coredatabase.Bind(app, db); err != nil {
			return err
		}
		loggerfacade.From(app).Info("数据库连接成功", zap.String(corelogger.TraceIDKey, ""))
		return nil
	}, func(context.Context) error {
		coredatabase.Close()
		return nil
	})
}
