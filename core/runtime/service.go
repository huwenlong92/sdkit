package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ServiceKind string

const (
	ServiceKindHTTP  ServiceKind = "http"
	ServiceKindQueue ServiceKind = "queue"
	ServiceKindCLI   ServiceKind = "cli"
)

type ServiceInfo struct {
	Name         string
	Type         string
	Kind         ServiceKind
	Addr         string
	Enabled      bool
	Capabilities []string
}

type RuntimeCapabilityInfo struct {
	Name     string
	Group    string
	Scope    string
	Internal bool
}

type ServiceInfoProvider interface {
	ServiceInfo() ServiceInfo
}

type Service interface {
	ServiceInfoProvider
	Start(context.Context) error
	Shutdown(context.Context) error
}

type ServiceFactory[T any] func(configFile string, name string, base T) (Service, error)
type ServiceContextFactory[T any] func(ctx ServiceContext[T]) (Service, error)
type ServiceRuntimeCapabilityFactory[T any] func(ctx RuntimeCapabilityContext[T]) []CapabilityContract

type ServiceDefinition[T any] struct {
	Type                     string
	Kind                     ServiceKind
	Factory                  ServiceFactory[T]
	ContextFactory           ServiceContextFactory[T]
	RuntimeCapabilityFactory ServiceRuntimeCapabilityFactory[T]
}

type serviceRegistration[T any] struct {
	Kind                     ServiceKind
	Factory                  ServiceFactory[T]
	ContextFactory           ServiceContextFactory[T]
	RuntimeCapabilityFactory ServiceRuntimeCapabilityFactory[T]
}

type ServiceRegistry[T any] struct {
	factories map[string]serviceRegistration[T]
}

func NewServiceRegistry[T any]() *ServiceRegistry[T] {
	return &ServiceRegistry[T]{
		factories: make(map[string]serviceRegistration[T]),
	}
}

func (r *ServiceRegistry[T]) RegisterServiceDefinition(def ServiceDefinition[T]) {
	if r == nil || def.Type == "" || (def.Factory == nil && def.ContextFactory == nil) {
		return
	}
	if r.factories == nil {
		r.factories = make(map[string]serviceRegistration[T])
	}
	r.factories[def.Type] = serviceRegistration[T]{
		Kind:                     def.Kind,
		Factory:                  def.Factory,
		ContextFactory:           def.ContextFactory,
		RuntimeCapabilityFactory: def.RuntimeCapabilityFactory,
	}
}

func (r *ServiceRegistry[T]) BuildServices(configFile string, specs map[string]ServiceSpec, base T) ([]Service, error) {
	if len(specs) == 0 {
		return nil, errors.New("runtime: services config is required")
	}
	services := make([]Service, 0, len(specs))
	names := make([]string, 0, len(specs))
	for name := range specs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		spec := specs[name]
		if spec.Enabled != nil && !*spec.Enabled {
			continue
		}
		if spec.Type == "" {
			return nil, errors.New("runtime: service " + name + " type is required")
		}
		svc, reg, err := r.buildService(configFile, name, spec.Type, spec.ResolveConfigKey(name), base, nil)
		if err != nil {
			return nil, err
		}
		services = append(services, withRegistrationInfo{
			Service:      svc,
			serviceType:  spec.Type,
			kind:         reg.Kind,
			capabilities: registeredCapabilityNames(svc),
		})
	}
	return services, nil
}

func (r *ServiceRegistry[T]) BuildService(configFile string, name string, serviceType string, configKey string, base T) (Service, error) {
	svc, reg, err := r.buildService(configFile, name, serviceType, configKey, base, nil)
	if err != nil {
		return nil, err
	}
	return withRegistrationInfo{
		Service:      svc,
		serviceType:  serviceType,
		kind:         reg.Kind,
		capabilities: registeredCapabilityNames(svc),
	}, nil
}

func (r *ServiceRegistry[T]) BuildServiceWithCapabilities(configFile string, name string, serviceType string, configKey string, base T, capabilities *LocalCapabilityRegistry) (Service, error) {
	svc, reg, err := r.buildService(configFile, name, serviceType, configKey, base, capabilities)
	if err != nil {
		return nil, err
	}
	return withRegistrationInfo{
		Service:      svc,
		serviceType:  serviceType,
		kind:         reg.Kind,
		capabilities: registeredCapabilityNames(svc),
	}, nil
}

func (r *ServiceRegistry[T]) RuntimeCapabilitiesForService(ctx RuntimeCapabilityContext[T]) []CapabilityContract {
	ctx = normalizeRuntimeCapabilityContext(ctx)
	if r == nil {
		return nil
	}
	reg := r.factories[ctx.Type]
	if reg.RuntimeCapabilityFactory == nil {
		return nil
	}
	capabilities := reg.RuntimeCapabilityFactory(ctx)
	if len(capabilities) == 0 {
		return nil
	}
	out := make([]CapabilityContract, 0, len(capabilities))
	for _, capability := range capabilities {
		if capability == nil {
			out = append(out, nil)
			continue
		}
		out = append(out, serviceLocalRuntimeCapability{
			CapabilityContract: capability,
			serviceType:        ctx.Type,
		})
	}
	return out
}

func (r *ServiceRegistry[T]) buildService(configFile string, name string, serviceType string, configKey string, base T, capabilities *LocalCapabilityRegistry) (Service, serviceRegistration[T], error) {
	if serviceType == "" {
		return nil, serviceRegistration[T]{}, errors.New("runtime: service " + name + " type is required")
	}
	if configKey == "" {
		configKey = name
	}
	if r == nil {
		return nil, serviceRegistration[T]{}, errors.New("runtime: unsupported service type " + serviceType)
	}
	reg := r.factories[serviceType]
	if reg.Factory == nil && reg.ContextFactory == nil {
		return nil, serviceRegistration[T]{}, errors.New("runtime: unsupported service type " + serviceType)
	}
	if capabilities == nil {
		capabilities = NewLocalCapabilityRegistry()
	}
	if reg.ContextFactory != nil {
		svc, err := reg.ContextFactory(ServiceContext[T]{
			ConfigFile:   configFile,
			Name:         name,
			Type:         serviceType,
			ConfigKey:    configKey,
			Base:         base,
			Capabilities: capabilities,
		})
		return attachCapabilities(svc, capabilities), reg, err
	}
	svc, err := reg.Factory(configFile, name, base)
	return svc, reg, err
}

func attachCapabilities(svc Service, registry *LocalCapabilityRegistry) Service {
	if svc == nil {
		return nil
	}
	return withRegisteredCapabilities{
		Service:      svc,
		capabilities: registry.Names(),
	}
}

type withRegisteredCapabilities struct {
	Service
	capabilities []string
}

func (s withRegisteredCapabilities) ServiceInfo() ServiceInfo {
	info := s.Service.ServiceInfo()
	info.Capabilities = mergeCapabilitiesForService(info.Name, info.Capabilities, s.capabilities)
	return info
}

type withRegistrationInfo struct {
	Service
	serviceType  string
	kind         ServiceKind
	capabilities []string
}

func (s withRegistrationInfo) ServiceInfo() ServiceInfo {
	info := s.Service.ServiceInfo()
	if info.Type == "" {
		info.Type = s.serviceType
	}
	if info.Kind == "" {
		info.Kind = s.kind
	}
	info.Capabilities = mergeCapabilitiesForService(info.Name, info.Capabilities, s.capabilities)
	return info
}

type ServiceCapabilityProvider interface {
	Capabilities() []LocalCapability
}

func registeredCapabilityNames(svc Service) []string {
	provider, ok := svc.(ServiceCapabilityProvider)
	if !ok {
		return nil
	}
	registry := NewLocalCapabilityRegistry()
	for _, capability := range provider.Capabilities() {
		registry.Add(capability)
	}
	return registry.Names()
}

func MergeCapabilities(values []string, extra []string) []string {
	return mergeCapabilitiesForService("", values, extra)
}

func MergeCapabilitiesForService(serviceName string, values []string, extra []string) []string {
	return mergeCapabilitiesForService(serviceName, values, extra)
}

func mergeCapabilitiesForService(serviceName string, values []string, extra []string) []string {
	if len(values) == 0 && len(extra) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values)+len(extra))
	out := make([]string, 0, len(values)+len(extra))
	for _, value := range append(values, extra...) {
		label, ok := displayCapabilityForService(serviceName, value)
		if !ok {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}

func displayCapabilityForService(serviceName string, value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	prefix := strings.TrimSpace(serviceName)
	if prefix != "" {
		if local, ok := strings.CutPrefix(value, prefix+"."); ok {
			local = strings.TrimSpace(local)
			if local == "" {
				return "", false
			}
			return local, true
		}
	}
	return value, true
}

type HTTPService struct {
	InfoValue ServiceInfo
	StartFunc func() error
	StopFunc  func(context.Context) error
}

func (s HTTPService) ServiceInfo() ServiceInfo {
	return s.InfoValue
}

func (s HTTPService) Start(context.Context) error {
	if s.StartFunc == nil {
		return nil
	}
	return s.StartFunc()
}

func (s HTTPService) Shutdown(ctx context.Context) error {
	if s.StopFunc == nil {
		return nil
	}
	return s.StopFunc(ctx)
}

type ServiceSpec struct {
	Type      string `mapstructure:"type" yaml:"type"`
	Enabled   *bool  `mapstructure:"enabled" yaml:"enabled"`
	ConfigKey string `mapstructure:"config_key" yaml:"config_key"`
}

func (s ServiceSpec) ResolveConfigKey(name string) string {
	if s.ConfigKey != "" {
		return s.ConfigKey
	}
	return name
}

type ServiceConfigError struct {
	ServiceName string
	ServiceType string
	ConfigKey   string
	Err         error
}

func (e ServiceConfigError) Error() string {
	return fmt.Sprintf(
		"runtime: load service config failed: service=%s type=%s config_key=%s: %v",
		e.ServiceName,
		e.ServiceType,
		e.ConfigKey,
		e.Err,
	)
}

func (e ServiceConfigError) Unwrap() error {
	return e.Err
}

func WrapServiceConfigError(serviceName string, serviceType string, configKey string, err error) error {
	if err == nil {
		return nil
	}
	if serviceType == "" {
		serviceType = serviceName
	}
	if configKey == "" {
		configKey = serviceName
	}
	return ServiceConfigError{
		ServiceName: serviceName,
		ServiceType: serviceType,
		ConfigKey:   configKey,
		Err:         err,
	}
}
