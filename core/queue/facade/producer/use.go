package producer

import (
	"context"
	"errors"

	corequeue "github.com/huwenlong92/sdkit/core/queue"
	redisfacade "github.com/huwenlong92/sdkit/core/redis/facade"
	"github.com/huwenlong92/sdkit/core/runtime"
)

var ErrProducerConfigRequired = errors.New("queue producer facade: config or client required")

type UseOption func(*useOptions)

type useOptions struct {
	name         runtime.Key
	config       Config
	hasConfig    bool
	configLoader ConfigLoader
	producer     Producer
	dependencies []runtime.Dependency
	runtimeOpts  []corequeue.RuntimeInstanceOption
	internal     bool
}

func WithName(name string) UseOption {
	return func(o *useOptions) {
		o.name = runtime.Key(name)
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

func WithProducer(producer Producer) UseOption {
	return func(o *useOptions) {
		o.producer = producer
	}
}

func WithClient(client Client) UseOption {
	return WithProducer(client)
}

func WithDependencies(deps ...runtime.Dependency) UseOption {
	return func(o *useOptions) {
		o.dependencies = append(o.dependencies, deps...)
	}
}

func WithRuntimeOptions(opts ...corequeue.RuntimeInstanceOption) UseOption {
	return func(o *useOptions) {
		o.runtimeOpts = append(o.runtimeOpts, opts...)
	}
}

func WithInternal() UseOption {
	return func(o *useOptions) {
		o.internal = true
	}
}

func Use(opts ...UseOption) runtime.Capability {
	o := useOptions{name: KeyQueue}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if o.name == "" {
		o.name = KeyQueue
	}

	dependencies := []runtime.Dependency{
		runtime.Optional("bootstrap"),
		runtime.Optional(string(redisfacade.KeyRedis)),
	}
	dependencies = append(dependencies, o.dependencies...)

	var registered Producer
	ownsProducer := false

	return runtime.NewCapabilityWithMetadataAndDependencies(runtime.CapabilityMetadata{
		Name:        string(o.name),
		Description: "Queue producer",
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

		producer := o.producer
		if producer == nil && hasConfig {
			client, err := corequeue.NewClient(config)
			if err != nil {
				return err
			}
			if len(o.runtimeOpts) > 0 {
				instanceOpts := []corequeue.RuntimeInstanceOption{corequeue.WithRuntimeMetadata(corequeue.RuntimeMetadataFromConfig("", "", config))}
				instanceOpts = append(instanceOpts, o.runtimeOpts...)
				producer = corequeue.NewRuntimeInstanceFromParts(corequeue.RuntimeParts{Client: client}, instanceOpts...)
			} else {
				producer = client
			}
			ownsProducer = true
		}
		if producer == nil {
			return ErrProducerConfigRequired
		}

		registered = producer
		if o.producer != nil {
			ownsProducer = true
		}
		return app.Container().Bind(o.name, producer)
	}, func(context.Context) error {
		if registered == nil {
			return nil
		}
		if ownsProducer {
			return registered.Close()
		}
		return nil
	})
}
