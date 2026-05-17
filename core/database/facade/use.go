package database

import (
	"context"

	coreconfig "github.com/huwenlong92/sdkit/core/config"
	coredatabase "github.com/huwenlong92/sdkit/core/database"
	corelogger "github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/runtime"
	tracingfacade "github.com/huwenlong92/sdkit/core/tracing/facade"

	"go.uber.org/zap"
)

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
		runtime.Optional(tracingfacade.Name),
	}
	dependencies = append(dependencies, o.dependencies...)

	return runtime.NewCapabilityWithMetadataAndDependencies(runtime.CapabilityMetadata{
		Name:        string(KeyDatabase),
		Description: "GORM and pgx database",
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
			if !hasConfig && coreconfig.V != nil {
				if err := coreconfig.V.UnmarshalKey("database", &config); err != nil {
					return err
				}
			}
			if mode == "" && coreconfig.V != nil {
				mode = coreconfig.V.GetString("app.mode")
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
		corelogger.From(app).Info("数据库连接成功", zap.String(corelogger.TraceIDKey, ""))
		return nil
	}, func(context.Context) error {
		coredatabase.Close()
		return nil
	})
}
