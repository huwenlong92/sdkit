package runtime

import "strings"

type RuntimeCapabilityContext[T any] struct {
	ConfigFile string
	Name       string
	Type       string
	ConfigKey  string
	Base       T

	baseLoader func() T
}

func (ctx RuntimeCapabilityContext[T]) LocalName(name string) string {
	name = strings.TrimSpace(name)
	serviceName := strings.TrimSpace(ctx.Name)
	if serviceName == "" {
		return name
	}
	if name == "" {
		return serviceName
	}
	if strings.HasPrefix(name, serviceName+".") {
		return name
	}
	return serviceName + "." + name
}

func (ctx RuntimeCapabilityContext[T]) BaseConfig() T {
	if ctx.baseLoader != nil {
		return ctx.baseLoader()
	}
	return ctx.Base
}

func NewRuntimeCapabilityContext[T any](configFile string, name string, serviceType string, configKey string, base T) RuntimeCapabilityContext[T] {
	return normalizeRuntimeCapabilityContext(RuntimeCapabilityContext[T]{
		ConfigFile: configFile,
		Name:       name,
		Type:       serviceType,
		ConfigKey:  configKey,
		Base:       base,
	})
}

func NewRuntimeCapabilityContextWithBaseLoader[T any](configFile string, name string, serviceType string, configKey string, baseLoader func() T) RuntimeCapabilityContext[T] {
	return normalizeRuntimeCapabilityContext(RuntimeCapabilityContext[T]{
		ConfigFile: configFile,
		Name:       name,
		Type:       serviceType,
		ConfigKey:  configKey,
		baseLoader: baseLoader,
	})
}

func normalizeRuntimeCapabilityContext[T any](ctx RuntimeCapabilityContext[T]) RuntimeCapabilityContext[T] {
	if ctx.Type == "" {
		ctx.Type = ctx.Name
	}
	if ctx.ConfigKey == "" {
		ctx.ConfigKey = ctx.Name
	}
	return ctx
}

type serviceLocalRuntimeCapability struct {
	CapabilityContract
	serviceType string
}

func (c serviceLocalRuntimeCapability) Name() string {
	return c.Metadata().Name
}

func (c serviceLocalRuntimeCapability) Metadata() CapabilityMetadata {
	metadata := c.CapabilityContract.Metadata()
	if metadata.Name == "" {
		metadata.Name = c.CapabilityContract.Name()
	}
	if metadata.Group == "" {
		metadata.Group = c.serviceType
	}
	metadata.Scope = ScopeServiceLocal
	return metadata
}
