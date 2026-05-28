package eventbus

import (
	"context"

	coreeventbus "github.com/huwenlong92/sdkit/core/eventbus"
	redisfacade "github.com/huwenlong92/sdkit/core/redis/facade"
	"github.com/huwenlong92/sdkit/core/runtime"

	goredis "github.com/redis/go-redis/v9"
)

type UseOption func(*useOptions)

type useOptions struct {
	config       Config
	configLoader ConfigLoader
	service      Service
	redis        *goredis.Client
	dependencies []runtime.Dependency
	internal     bool
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

func WithService(service Service) UseOption {
	return func(o *useOptions) {
		o.service = service
	}
}

func UseWithRedisClient(redis *goredis.Client) UseOption {
	return func(o *useOptions) {
		o.redis = redis
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
		runtime.Optional(redisfacade.Name),
	}
	dependencies = append(dependencies, o.dependencies...)

	var runtimeService Service
	return runtime.NewCapabilityWithMetadataAndDependencies(runtime.CapabilityMetadata{
		Name:        Name,
		Description: "EventBus service",
		Group:       runtime.GroupSystem,
		Scope:       runtime.ScopeGlobal,
		Internal:    o.internal,
	}, dependencies, func(app *runtime.App) error {
		config := o.config
		if o.configLoader != nil {
			loaded, err := o.configLoader(app)
			if err != nil {
				return err
			}
			config = loaded
		}

		service := o.service
		if service == nil {
			redisClient := o.redis
			if redisClient == nil {
				if client := redisfacade.From(app); client != nil {
					redisClient = client.Rdb
				}
			}
			var err error
			service, err = New(config, WithRedisClient(redisClient))
			if err != nil {
				return err
			}
		}
		runtimeService = service
		return coreeventbus.BindWithDriver(app, service, service.Driver())
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
			if bus, ok := value.(coreeventbus.Service); ok {
				return attachDefaultService{bus: bus}
			}
		}
	}
	bus, _ := coreeventbus.DefaultWithDriver()
	if bus == nil {
		return nil
	}
	return attachDefaultService{bus: bus}
}

type attachDefaultService struct {
	bus coreeventbus.Bus
}

func (s attachDefaultService) Publish(ctx context.Context, event *coreeventbus.Event) error {
	return s.bus.Publish(ctx, event)
}

func (s attachDefaultService) Subscribe(ctx context.Context, topic string, handler coreeventbus.Handler) (coreeventbus.Subscription, error) {
	return s.bus.Subscribe(ctx, topic, handler)
}

func (s attachDefaultService) Close() error {
	return nil
}

func (s attachDefaultService) Capability() coreeventbus.Capability {
	return s.bus.Capability()
}

func (s attachDefaultService) Bus() coreeventbus.Bus {
	return s.bus
}

func (s attachDefaultService) Driver() string {
	return coreeventbus.DefaultDriver()
}

func closeService(service *Service) error {
	if service == nil || *service == nil {
		return nil
	}
	current := *service
	*service = nil
	return current.Close()
}
