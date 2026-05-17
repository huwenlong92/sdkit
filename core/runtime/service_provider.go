package runtime

type ServiceProvider[T any] interface {
	Register(app *ServiceApp[T]) error
}

type ServiceProviderFunc[T any] func(app *ServiceApp[T]) error

func (fn ServiceProviderFunc[T]) Register(app *ServiceApp[T]) error {
	return fn(app)
}

type ServiceApp[T any] struct {
	registry *ServiceRegistry[T]
}

func NewServiceApp[T any](registry *ServiceRegistry[T]) *ServiceApp[T] {
	if registry == nil {
		registry = NewServiceRegistry[T]()
	}
	return &ServiceApp[T]{
		registry: registry,
	}
}

func (a *ServiceApp[T]) Service(serviceType string) *ServiceBuilder[T] {
	registry := (*ServiceRegistry[T])(nil)
	if a != nil {
		registry = a.registry
	}
	if registry == nil {
		registry = NewServiceRegistry[T]()
	}
	return &ServiceBuilder[T]{
		registry: registry,
		def:      ServiceDefinition[T]{Type: serviceType},
	}
}

func RegisterServiceProvider[T any](registry *ServiceRegistry[T], provider ServiceProvider[T]) error {
	if provider == nil {
		return nil
	}
	return provider.Register(NewServiceApp(registry))
}

type ServiceBuilder[T any] struct {
	registry *ServiceRegistry[T]
	def      ServiceDefinition[T]
}

func (b *ServiceBuilder[T]) Kind(kind ServiceKind) *ServiceBuilder[T] {
	b.def.Kind = kind
	return b
}

func (b *ServiceBuilder[T]) RuntimeCapabilities(factory ServiceRuntimeCapabilityFactory[T]) *ServiceBuilder[T] {
	b.def.RuntimeCapabilityFactory = factory
	return b
}

func (b *ServiceBuilder[T]) Factory(factory ServiceFactory[T]) {
	b.def.Factory = factory
	b.register()
}

func (b *ServiceBuilder[T]) FactoryContext(factory ServiceContextFactory[T]) {
	b.def.ContextFactory = factory
	b.register()
}

func (b *ServiceBuilder[T]) register() {
	if b == nil {
		return
	}
	if b.registry == nil {
		b.registry = NewServiceRegistry[T]()
	}
	b.registry.RegisterServiceDefinition(b.def)
}
