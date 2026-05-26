package operations

import (
	"context"
	"errors"

	corequeue "github.com/huwenlong92/sdkit/core/queue"
	redisfacade "github.com/huwenlong92/sdkit/core/redis/facade"
	"github.com/huwenlong92/sdkit/core/runtime"
)

var ErrOperationsConfigRequired = errors.New("queue operations facade: config required")

type UseOption func(*useOptions)

type useOptions struct {
	name         runtime.Key
	config       Config
	hasConfig    bool
	configLoader ConfigLoader
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
	o := useOptions{name: runtime.Key(Name)}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if o.name == "" {
		o.name = runtime.Key(Name)
	}

	dependencies := []runtime.Dependency{
		runtime.OptionalBootstrap(),
		runtime.Optional(redisfacade.Name),
	}
	dependencies = append(dependencies, o.dependencies...)

	var registered *corequeue.RuntimeInstance
	return runtime.NewCapabilityWithMetadataAndDependencies(runtime.CapabilityMetadata{
		Name:        string(o.name),
		Description: "Queue operations",
		Group:       runtime.GroupSystem,
		Scope:       runtime.ScopeGlobal,
		Internal:    o.internal,
	}, dependencies, func(app *runtime.App) error {
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
			return ErrOperationsConfigRequired
		}
		queueCfg := cfg.Queue
		client, err := corequeue.NewClient(queueCfg)
		if err != nil {
			return err
		}
		manager, err := corequeue.NewManager(queueCfg)
		if err != nil {
			_ = client.Close()
			return err
		}
		metadata := cfg.Metadata
		if isZeroMetadata(metadata) {
			metadata = corequeue.RuntimeMetadataFromConfig("", "", queueCfg)
		}
		instanceOpts := []corequeue.RuntimeInstanceOption{corequeue.WithRuntimeMetadata(metadata)}
		instanceOpts = append(instanceOpts, o.runtimeOpts...)
		registered = corequeue.NewRuntimeInstanceFromParts(corequeue.RuntimeParts{Client: client, Manager: manager}, instanceOpts...)
		return app.Container().Bind(o.name, registered)
	}, func(context.Context) error {
		if registered == nil {
			return nil
		}
		return registered.Close()
	})
}

func isZeroMetadata(metadata corequeue.RuntimeMetadata) bool {
	return metadata.Name == "" &&
		metadata.Service == "" &&
		metadata.Driver == "" &&
		len(metadata.Queues) == 0 &&
		metadata.DefaultQueue == "" &&
		metadata.Concurrency == 0
}
