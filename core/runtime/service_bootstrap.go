package runtime

import "errors"

var ErrServiceSpecLoaderRequired = errors.New("runtime: service specs loader is required")

type ServiceSpecLoader func(configFile string) (map[string]ServiceSpec, error)
type ServiceSpecResolver func(configFile string, name string) (ServiceSpec, error)

type ServiceBootstrap[T any] struct {
	registry    *ServiceRegistry[T]
	loadSpecs   ServiceSpecLoader
	resolveSpec ServiceSpecResolver
}

func NewServiceBootstrap[T any](loadSpecs ServiceSpecLoader) *ServiceBootstrap[T] {
	return NewServiceBootstrapWithResolver[T](loadSpecs, nil)
}

func NewServiceBootstrapWithResolver[T any](loadSpecs ServiceSpecLoader, resolveSpec ServiceSpecResolver) *ServiceBootstrap[T] {
	return &ServiceBootstrap[T]{
		registry:    NewServiceRegistry[T](),
		loadSpecs:   loadSpecs,
		resolveSpec: resolveSpec,
	}
}

func (b *ServiceBootstrap[T]) Registry() *ServiceRegistry[T] {
	if b == nil {
		return nil
	}
	if b.registry == nil {
		b.registry = NewServiceRegistry[T]()
	}
	return b.registry
}

func (b *ServiceBootstrap[T]) App() *ServiceApp[T] {
	if b == nil {
		return NewServiceApp[T](nil)
	}
	return NewServiceApp(b.Registry())
}

func (b *ServiceBootstrap[T]) Service(serviceType string) *ServiceBuilder[T] {
	return b.App().Service(serviceType)
}

func (b *ServiceBootstrap[T]) RegisterServiceDefinition(def ServiceDefinition[T]) {
	registry := b.Registry()
	if registry == nil {
		return
	}
	registry.RegisterServiceDefinition(def)
}

func (b *ServiceBootstrap[T]) RegisterProvider(provider ServiceProvider[T]) error {
	return RegisterServiceProvider(b.Registry(), provider)
}

func (b *ServiceBootstrap[T]) LoadServiceSpecs(configFile string) (map[string]ServiceSpec, error) {
	if b == nil || b.loadSpecs == nil {
		return nil, ErrServiceSpecLoaderRequired
	}
	return b.loadSpecs(configFile)
}

func (b *ServiceBootstrap[T]) ResolveServiceSpec(configFile string, name string) (ServiceSpec, error) {
	if name == "" {
		return ServiceSpec{}, nil
	}
	if b != nil && b.resolveSpec != nil {
		return b.resolveSpec(configFile, name)
	}
	specs, err := b.LoadServiceSpecs(configFile)
	if err != nil {
		return ServiceSpec{}, err
	}
	return specs[name], nil
}

func (b *ServiceBootstrap[T]) ResolveServiceConfigKey(configFile string, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	spec, err := b.ResolveServiceSpec(configFile, name)
	if err != nil {
		return "", err
	}
	return spec.ResolveConfigKey(name), nil
}

func (b *ServiceBootstrap[T]) BuildServices(configFile string, base T) ([]Service, error) {
	specs, err := b.LoadServiceSpecs(configFile)
	if err != nil {
		return nil, err
	}
	return b.Registry().BuildServices(configFile, specs, base)
}

func (b *ServiceBootstrap[T]) BuildService(configFile string, name string, serviceType string, base T) (Service, error) {
	configKey, err := b.ResolveServiceConfigKey(configFile, name)
	if err != nil {
		return nil, err
	}
	return b.Registry().BuildService(configFile, name, serviceType, configKey, base)
}

func (b *ServiceBootstrap[T]) BuildServiceWithCapabilities(configFile string, name string, serviceType string, base T, capabilities *LocalCapabilityRegistry) (Service, error) {
	configKey, err := b.ResolveServiceConfigKey(configFile, name)
	if err != nil {
		return nil, err
	}
	return b.Registry().BuildServiceWithCapabilities(configFile, name, serviceType, configKey, base, capabilities)
}

func (b *ServiceBootstrap[T]) RuntimeCapabilitiesForService(ctx RuntimeCapabilityContext[T]) []CapabilityContract {
	registry := b.Registry()
	if registry == nil {
		return nil
	}
	return registry.RuntimeCapabilitiesForService(ctx)
}

func (b *ServiceBootstrap[T]) ServiceKindForType(serviceType string) (ServiceKind, bool) {
	registry := b.Registry()
	if registry == nil {
		return "", false
	}
	return registry.ServiceKind(serviceType)
}
