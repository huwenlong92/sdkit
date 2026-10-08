// Package facade binds system probes to a service runtime.
package facade

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/huwenlong92/sdkit/core/config"
	"github.com/huwenlong92/sdkit/core/runtime"
	"github.com/huwenlong92/sdkit/core/sysprobe"
)

const Name = "sysprobe"

type Config = sysprobe.Config
type HostConfig = sysprobe.HostConfig
type NetworkCapacityConfig = sysprobe.NetworkCapacityConfig
type DependencySpec = sysprobe.DependencySpec
type Service = sysprobe.Service
type ConfigLoader func(*runtime.App) (Config, error)
type Option func(*options)

type options struct {
	name        string
	loader      ConfigLoader
	bindDefault bool
}

var defaultService atomic.Pointer[Service]

func WithName(name string) Option {
	return func(o *options) {
		if name != "" {
			o.name = name
		}
	}
}

func WithConfig(cfg Config) Option {
	return WithConfigLoader(func(*runtime.App) (Config, error) { return cfg, nil })
}

func WithConfigLoader(loader ConfigLoader) Option {
	return func(o *options) { o.loader = loader }
}

// WithDefault makes this instance the process default only if no default exists.
func WithDefault() Option {
	return func(o *options) { o.bindDefault = true }
}

func Use(opts ...Option) runtime.Capability {
	o := options{name: Name}
	for _, option := range opts {
		if option != nil {
			option(&o)
		}
	}
	var mu sync.Mutex
	var service *Service
	return runtime.NewCapabilityWithMetadata(runtime.CapabilityMetadata{
		Name: o.name, Description: "Host and executable dependency probe",
		Group: runtime.GroupSystem, Scope: runtime.ScopeServiceLocal,
	}, func(app *runtime.App) error {
		mu.Lock()
		defer mu.Unlock()
		if app == nil {
			return fmt.Errorf("sysprobe: runtime app is required")
		}
		if service == nil {
			if o.loader == nil {
				return fmt.Errorf("sysprobe: config loader is required")
			}
			cfg, err := o.loader(app)
			if err != nil {
				return err
			}
			service = sysprobe.New(cfg)
		}
		if err := app.Container().Bind(runtime.Key(o.name), service); err != nil {
			return err
		}
		if o.bindDefault {
			defaultService.CompareAndSwap(nil, service)
		}
		return nil
	}, func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		defaultService.CompareAndSwap(service, nil)
		if service != nil {
			if err := service.Close(); err != nil {
				return err
			}
		}
		service = nil
		return nil
	})
}

func UseConfigFile(path string, opts ...Option) runtime.Capability {
	loader := WithConfigLoader(func(*runtime.App) (Config, error) {
		var cfg Config
		if err := config.LoadRequiredKey(path, Name, &cfg); err != nil {
			return Config{}, err
		}
		return cfg, nil
	})
	return Use(append([]Option{loader}, opts...)...)
}

func From(app *runtime.App, names ...string) *Service {
	if app == nil {
		return nil
	}
	name := Name
	if len(names) > 0 && names[0] != "" {
		name = names[0]
	}
	value, ok := app.Container().Get(runtime.Key(name))
	if !ok {
		return nil
	}
	service, _ := value.(*Service)
	return service
}

func FromDefault() *Service { return defaultService.Load() }

func FromServiceContext[T any](ctx *runtime.ServiceContext[T]) *Service {
	if ctx == nil {
		return nil
	}
	value, ok := ctx.CapabilityLocalFirst(Name)
	if !ok {
		return nil
	}
	service, _ := value.(*Service)
	return service
}
