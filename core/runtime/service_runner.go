package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ServiceRunner[T any] struct {
	bootstrap *ServiceBootstrap[T]
	options   ServiceRunnerOptions[T]
}

type ServiceRunnerOptions[T any] struct {
	Bootstrap           *ServiceBootstrap[T]
	LoadSpecs           ServiceSpecLoader
	ResolveSpec         ServiceSpecResolver
	Providers           []ServiceProvider[T]
	Capabilities        func(ServiceSelection) []CapabilityContract
	BaseConfig          func() T
	Dependencies        func(ServiceSelectionItem) []Dependency
	Group               func(ServiceSelectionItem) string
	CapabilityRegistry  func(*App) *LocalCapabilityRegistry
	CapabilityInfos     func(*App) []RuntimeCapabilityInfo
	NormalizeType       func(string) string
	AllowService        func(ServiceSelectionItem) bool
	OnServiceRegistered func(ServiceRegistered[T])
}

type ServiceRunOptions struct {
	ConfigFile string
	Services   []string
}

type ServiceSelection struct {
	ConfigFile string
	Services   []ServiceSelectionItem
}

type ServiceSelectionItem struct {
	ConfigFile   string
	Name         string
	Type         string
	ConfigKey    string
	Kind         ServiceKind
	Group        string
	Dependencies []Dependency
	Spec         ServiceSpec
	Explicit     bool
}

type ServiceRegistered[T any] struct {
	App          *App
	Base         T
	Service      Service
	ServiceInfo  ServiceInfo
	Capabilities []RuntimeCapabilityInfo
	Selection    ServiceSelectionItem
}

func NewServiceRunner[T any](options ServiceRunnerOptions[T]) *ServiceRunner[T] {
	bootstrap := options.Bootstrap
	if bootstrap == nil {
		bootstrap = NewServiceBootstrapWithResolver[T](options.LoadSpecs, options.ResolveSpec)
	}
	return &ServiceRunner[T]{
		bootstrap: bootstrap,
		options:   options,
	}
}

func (r *ServiceRunner[T]) Bootstrap() *ServiceBootstrap[T] {
	if r == nil {
		return nil
	}
	if r.bootstrap == nil {
		r.bootstrap = NewServiceBootstrapWithResolver[T](r.options.LoadSpecs, r.options.ResolveSpec)
	}
	return r.bootstrap
}

func (r *ServiceRunner[T]) RegisterProviders(providers ...ServiceProvider[T]) error {
	bootstrap := r.Bootstrap()
	if bootstrap == nil {
		return ErrServiceSpecLoaderRequired
	}
	for _, provider := range providers {
		if err := bootstrap.RegisterProvider(provider); err != nil {
			return err
		}
	}
	return nil
}

func (r *ServiceRunner[T]) NewApp(options ServiceRunOptions) (*App, error) {
	selection, err := r.SelectServices(options)
	if err != nil {
		return nil, err
	}
	app := New()
	if r.options.Capabilities != nil {
		if err := app.RegisterCapabilities(r.options.Capabilities(selection)...); err != nil {
			return nil, err
		}
	}
	providers := make([]ProviderContract, 0, len(selection.Services))
	for _, item := range selection.Services {
		providers = append(providers, &managedServiceProvider[T]{
			runner:    r,
			selection: item,
		})
	}
	if len(providers) > 0 {
		if err := app.Register(providers...); err != nil {
			return nil, err
		}
	}
	return app, nil
}

func (r *ServiceRunner[T]) SelectServices(options ServiceRunOptions) (ServiceSelection, error) {
	bootstrap := r.Bootstrap()
	if bootstrap == nil {
		return ServiceSelection{}, ErrServiceSpecLoaderRequired
	}
	if err := r.RegisterProviders(r.options.Providers...); err != nil {
		return ServiceSelection{}, err
	}
	specs, err := bootstrap.LoadServiceSpecs(options.ConfigFile)
	if err != nil {
		return ServiceSelection{}, err
	}
	explicit := len(options.Services) > 0
	names := serviceSelectionNames(specs, options.Services)
	items := make([]ServiceSelectionItem, 0, len(names))
	for _, name := range names {
		spec, ok := specs[name]
		if !ok {
			return ServiceSelection{}, fmt.Errorf("%w: %s", ErrProviderNotFound, name)
		}
		if !explicit && spec.Enabled != nil && !*spec.Enabled {
			continue
		}
		if spec.Type == "" {
			return ServiceSelection{}, errors.New("runtime: service " + name + " type is required")
		}
		serviceType := r.normalizeType(spec.Type)
		configKey := spec.ResolveConfigKey(name)
		item := ServiceSelectionItem{
			ConfigFile: options.ConfigFile,
			Name:       name,
			Type:       serviceType,
			ConfigKey:  configKey,
			Spec:       spec,
			Explicit:   explicit,
		}
		if r.options.AllowService != nil && !r.options.AllowService(item) {
			if explicit {
				return ServiceSelection{}, fmt.Errorf("%w: %s", ErrProviderNotFound, name)
			}
			continue
		}
		kind, ok := bootstrap.ServiceKindForType(serviceType)
		if !ok {
			return ServiceSelection{}, errors.New("runtime: unsupported service type " + serviceType)
		}
		item.Kind = kind
		item.Group = bootstrap.ServiceGroupForType(serviceType)
		item.Dependencies = bootstrap.ServiceDependenciesForType(serviceType)
		items = append(items, item)
	}
	return ServiceSelection{
		ConfigFile: options.ConfigFile,
		Services:   items,
	}, nil
}

func (r *ServiceRunner[T]) ResolveServiceType(configFile string, name string) (string, error) {
	if name == "" {
		return "", ErrProviderNameRequired
	}
	spec, err := r.Bootstrap().ResolveServiceSpec(configFile, name)
	if err != nil {
		return "", err
	}
	if spec.Type == "" {
		return "", errors.New("runtime: service " + name + " type is required")
	}
	return r.normalizeType(spec.Type), nil
}

func (r *ServiceRunner[T]) normalizeType(serviceType string) string {
	if r != nil && r.options.NormalizeType != nil {
		return r.options.NormalizeType(serviceType)
	}
	return serviceType
}

func (r *ServiceRunner[T]) baseConfig() T {
	if r != nil && r.options.BaseConfig != nil {
		return r.options.BaseConfig()
	}
	var zero T
	return zero
}

func serviceSelectionNames(specs map[string]ServiceSpec, requested []string) []string {
	if len(requested) > 0 {
		names := make([]string, 0, len(requested))
		seen := make(map[string]struct{}, len(requested))
		for _, name := range requested {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
		return names
	}
	names := make([]string, 0, len(specs))
	for name := range specs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type managedServiceProvider[T any] struct {
	runner    *ServiceRunner[T]
	selection ServiceSelectionItem
	service   Service
}

func (p *managedServiceProvider[T]) Name() string {
	return p.selection.Name
}

func (p *managedServiceProvider[T]) Metadata() ProviderMetadata {
	group := p.selection.Group
	if group == "" {
		group = p.selection.Type
	}
	if p.runner != nil && p.runner.options.Group != nil {
		if resolved := p.runner.options.Group(p.selection); resolved != "" {
			group = resolved
		}
	}
	return ProviderMetadata{
		Name:  p.selection.Name,
		Group: group,
		Mode:  ProviderModeService,
	}
}

func (p *managedServiceProvider[T]) Dependencies() []Dependency {
	var deps []Dependency
	if p.runner != nil && p.runner.options.Dependencies != nil {
		deps = append(deps, p.runner.options.Dependencies(p.selection)...)
	}
	deps = append(deps, p.selection.Dependencies...)
	for _, capability := range p.RuntimeCapabilities() {
		name := capabilityName(capability)
		if name == "" {
			continue
		}
		deps = append(deps, Require(name))
	}
	return dedupeServiceDependencies(deps)
}

func (p *managedServiceProvider[T]) Register(app *App) error {
	if p.runner == nil || p.runner.Bootstrap() == nil {
		return ErrProviderNil
	}
	base := p.runner.baseConfig()
	svc, err := p.runner.Bootstrap().BuildServiceWithCapabilities(
		p.selection.ConfigFile,
		p.selection.Name,
		p.selection.Type,
		base,
		p.serviceCapabilities(app),
	)
	if err != nil {
		return err
	}
	p.service = svc
	if p.runner.options.OnServiceRegistered != nil {
		info := svc.ServiceInfo()
		info.Capabilities = ServiceRuntimeCapabilityNames(p.selection.Name, p.RuntimeCapabilities())
		p.runner.options.OnServiceRegistered(ServiceRegistered[T]{
			App:          app,
			Base:         base,
			Service:      svc,
			ServiceInfo:  info,
			Capabilities: p.capabilityInfos(app),
			Selection:    p.selection,
		})
	}
	return nil
}

func (p *managedServiceProvider[T]) Start(ctx context.Context) error {
	if p.service == nil {
		return fmt.Errorf("runtime: service %s is not registered", p.selection.Name)
	}
	info := p.service.ServiceInfo()
	if !info.Enabled {
		if ctx == nil {
			ctx = context.Background()
		}
		<-ctx.Done()
		return nil
	}
	err := p.service.Start(ctx)
	if err != nil && ctx != nil && ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return nil
	}
	return err
}

func (p *managedServiceProvider[T]) Stop(ctx context.Context) error {
	if p.service == nil {
		return nil
	}
	return p.service.Shutdown(ctx)
}

func (p *managedServiceProvider[T]) RuntimeManaged() bool {
	return true
}

func (p *managedServiceProvider[T]) RuntimeCapabilities() []CapabilityContract {
	if p == nil || p.runner == nil || p.runner.Bootstrap() == nil {
		return nil
	}
	return p.runner.Bootstrap().RuntimeCapabilitiesForService(
		NewRuntimeCapabilityContextWithBaseLoader(
			p.selection.ConfigFile,
			p.selection.Name,
			p.selection.Type,
			p.selection.ConfigKey,
			p.runner.baseConfig,
		),
	)
}

func (p *managedServiceProvider[T]) serviceCapabilities(app *App) *LocalCapabilityRegistry {
	var registry *LocalCapabilityRegistry
	if p.runner != nil && p.runner.options.CapabilityRegistry != nil {
		registry = p.runner.options.CapabilityRegistry(app)
	}
	if registry == nil {
		registry = NewLocalCapabilityRegistry()
	}
	for _, capability := range p.RuntimeCapabilities() {
		name := capabilityName(capability)
		if name == "" {
			continue
		}
		if app != nil {
			if value, ok := app.Container().Get(Key(name)); ok {
				registry.Set(name, value)
				continue
			}
		}
		registry.AddName(name)
	}
	return registry
}

func (p *managedServiceProvider[T]) capabilityInfos(app *App) []RuntimeCapabilityInfo {
	if p.runner != nil && p.runner.options.CapabilityInfos != nil {
		return p.runner.options.CapabilityInfos(app)
	}
	return RuntimeCapabilityInfos(app)
}

func RuntimeCapabilityInfos(app *App) []RuntimeCapabilityInfo {
	if app == nil {
		return nil
	}
	capabilities := app.Capabilities()
	if len(capabilities) == 0 {
		return nil
	}
	infos := make([]RuntimeCapabilityInfo, 0, len(capabilities))
	for _, capability := range capabilities {
		info, ok := RuntimeCapabilityInfoFor(capability)
		if !ok {
			continue
		}
		infos = append(infos, info)
	}
	return infos
}

func RuntimeCapabilityInfoFor(capability CapabilityContract) (RuntimeCapabilityInfo, bool) {
	if capability == nil {
		return RuntimeCapabilityInfo{}, false
	}
	metadata := capability.Metadata()
	name := metadata.Name
	if name == "" {
		name = capability.Name()
	}
	if name == "" {
		return RuntimeCapabilityInfo{}, false
	}
	scope := metadata.Scope
	if scope == "" {
		scope = ScopeGlobal
	}
	return RuntimeCapabilityInfo{
		Name:     name,
		Group:    metadata.Group,
		Scope:    scope,
		Internal: metadata.Internal,
	}, true
}

func ServiceRuntimeCapabilityNames(serviceName string, capabilities []CapabilityContract) []string {
	if len(capabilities) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(capabilities))
	names := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		info, ok := RuntimeCapabilityInfoFor(capability)
		if !ok || info.Scope != ScopeServiceLocal {
			continue
		}
		name := trimServiceCapabilityName(serviceName, info.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

func trimServiceCapabilityName(serviceName string, name string) string {
	name = strings.TrimSpace(name)
	serviceName = strings.TrimSpace(serviceName)
	if name == "" || serviceName == "" {
		return name
	}
	prefix := serviceName + "."
	if strings.HasPrefix(name, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(name, prefix))
	}
	return name
}

func dedupeServiceDependencies(deps []Dependency) []Dependency {
	if len(deps) == 0 {
		return nil
	}
	out := make([]Dependency, 0, len(deps))
	seen := make(map[string]struct{}, len(deps))
	for _, dep := range deps {
		if dep.Name == "" {
			out = append(out, dep)
			continue
		}
		if _, ok := seen[dep.Name]; ok {
			continue
		}
		seen[dep.Name] = struct{}{}
		out = append(out, dep)
	}
	return out
}
