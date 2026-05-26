package payment

import (
	"context"

	corepayment "github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/core/runtime"
)

type UseOption func(*useOptions)

type useOptions struct {
	config        Config
	hasConfig     bool
	configLoader  ConfigLoader
	service       *Service
	registry      *Registry
	adapters      []ProviderAdapter
	setup         SetupFunc
	pricingPolicy PricingPolicy
	selector      ChannelSelector
	bindings      []ChannelBinding
	dependencies  []runtime.Dependency
	internal      bool
}

type SetupFunc func(app *runtime.App, registry *Registry) error

func defaultUseOptions() useOptions {
	return useOptions{
		configLoader: loadConfigFromCore,
		dependencies: []runtime.Dependency{
			runtime.Optional("bootstrap"),
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

func WithService(service *Service) UseOption {
	return func(o *useOptions) {
		o.service = service
	}
}

func WithRegistry(registry *Registry) UseOption {
	return func(o *useOptions) {
		o.registry = registry
	}
}

func WithAdapters(adapters ...ProviderAdapter) UseOption {
	return func(o *useOptions) {
		o.adapters = append(o.adapters, adapters...)
	}
}

func WithSetup(setup SetupFunc) UseOption {
	return func(o *useOptions) {
		o.setup = setup
	}
}

func WithPricingPolicy(policy PricingPolicy) UseOption {
	return func(o *useOptions) {
		o.pricingPolicy = policy
	}
}

func WithChannelSelector(selector ChannelSelector) UseOption {
	return func(o *useOptions) {
		o.selector = selector
	}
}

func WithChannelBindings(bindings ...ChannelBinding) UseOption {
	return func(o *useOptions) {
		o.bindings = append(o.bindings, bindings...)
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
		Description: "Payment service",
		Group:       runtime.GroupSystem,
		Scope:       runtime.ScopeGlobal,
		Internal:    o.internal,
	}, append([]runtime.Dependency(nil), o.dependencies...), func(app *runtime.App) error {
		service := o.service
		if service == nil {
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
			registry := o.registry
			if registry == nil {
				registry = corepayment.NewRegistry()
			}
			if o.setup != nil {
				if err := o.setup(app, registry); err != nil {
					return err
				}
			}
			for _, adapter := range o.adapters {
				if err := registry.Register(adapter); err != nil {
					return err
				}
			}
			selector := o.selector
			bindings := append([]ChannelBinding(nil), o.bindings...)
			if hasConfig {
				bindings = append(cfg.channelBindings(), bindings...)
			}
			if selector == nil && len(bindings) > 0 {
				var err error
				selector, err = corepayment.NewStaticChannelSelector(bindings)
				if err != nil {
					return err
				}
			}
			var err error
			service, err = corepayment.NewService(corepayment.ServiceConfig{
				Registry:        registry,
				PricingPolicy:   o.pricingPolicy,
				ChannelSelector: selector,
			})
			if err != nil {
				return err
			}
		}
		return corepayment.Bind(app, service)
	}, func(context.Context) error {
		corepayment.Close()
		return nil
	})
}
