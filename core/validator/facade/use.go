package validator

import (
	"context"

	"github.com/huwenlong92/sdkit/core/runtime"
	corevalidator "github.com/huwenlong92/sdkit/core/validator"
)

const (
	Name         = "validator"
	KeyValidator = runtime.Key(Name)
)

type UseOption func(*useOptions)

type useOptions struct {
	dependencies []runtime.Dependency
	internal     bool
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
	}
	dependencies = append(dependencies, o.dependencies...)

	return runtime.NewCapabilityWithMetadataAndDependencies(runtime.CapabilityMetadata{
		Name:        Name,
		Description: "Gin validator bindings",
		Group:       runtime.GroupSystem,
		Scope:       runtime.ScopeGlobal,
		Internal:    o.internal,
	}, dependencies, func(app *runtime.App) error {
		if err := corevalidator.Init(); err != nil {
			return err
		}
		return app.Container().Bind(KeyValidator, struct{}{})
	}, func(context.Context) error {
		return nil
	})
}
