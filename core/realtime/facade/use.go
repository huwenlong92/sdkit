package realtime

import (
	"context"

	coreconfig "github.com/huwenlong92/sdkit/core/config"
	eventbusfacade "github.com/huwenlong92/sdkit/core/eventbus/facade"
	corerealtime "github.com/huwenlong92/sdkit/core/realtime"
	"github.com/huwenlong92/sdkit/core/runtime"
)

type ConfigLoader func(app *runtime.App) (Config, error)

type UseOption func(*useOptions)

type useOptions struct {
	config       Config
	hasConfig    bool
	configLoader ConfigLoader
	service      Service
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

func WithService(service Service) UseOption {
	return func(o *useOptions) {
		o.service = service
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
		runtime.OptionalBootstrap(),
		runtime.Require(eventbusfacade.Name),
	}
	dependencies = append(dependencies, o.dependencies...)

	var runtimeService Service
	return runtime.NewCapabilityWithMetadataAndDependencies(runtime.CapabilityMetadata{
		Name:        Name,
		Description: "Realtime publisher",
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
		if !hasConfig && coreconfig.V != nil {
			if !coreconfig.V.IsSet("eventbus") {
				return corerealtime.ValidatePublisherConfig(corerealtime.PublisherConfig{})
			}
			var publisherConfig corerealtime.PublisherConfig
			if err := coreconfig.V.UnmarshalKey("eventbus", &publisherConfig); err != nil {
				return err
			}
			if err := corerealtime.ValidatePublisherConfig(publisherConfig); err != nil {
				return err
			}
			config.Topic = publisherConfig.Topic
			hasConfig = true
		}

		service := o.service
		if service == nil {
			eventbusService := eventbusfacade.From(app)
			if eventbusService == nil || eventbusService.Bus() == nil {
				return ErrEventBusNotConfigured
			}
			var err error
			service, err = New(config, WithEventBus(eventbusService.Bus()))
			if err != nil {
				return err
			}
		}
		runtimeService = service
		return corerealtime.Bind(app, service)
	}, func(context.Context) error {
		return closeService(&runtimeService)
	})
}

func From(app *runtime.App) Service {
	if app != nil {
		if value, ok := app.Container().Get(runtime.Key(Name)); ok {
			if service, ok := value.(Service); ok {
				return service
			}
		}
	}
	return Default()
}

func closeService(service *Service) error {
	if service == nil || *service == nil {
		return nil
	}
	current := *service
	*service = nil
	return current.Close()
}
