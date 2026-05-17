package runtime

import "strings"

type ServiceContext[T any] struct {
	ConfigFile   string
	Name         string
	Type         string
	ConfigKey    string
	Base         T
	Capabilities *LocalCapabilityRegistry
}

func NewServiceContext[T any](name string, serviceType string, base T) *ServiceContext[T] {
	return &ServiceContext[T]{
		Name:         name,
		Type:         serviceType,
		ConfigKey:    name,
		Base:         base,
		Capabilities: NewLocalCapabilityRegistry(),
	}
}

func EnsureServiceContext[T any](ctx *ServiceContext[T], name string, serviceType string, base T) *ServiceContext[T] {
	if ctx == nil {
		return NewServiceContext(name, serviceType, base)
	}
	if ctx.Name == "" {
		ctx.Name = name
	}
	if ctx.Type == "" {
		ctx.Type = serviceType
	}
	if ctx.ConfigKey == "" {
		ctx.ConfigKey = ctx.Name
	}
	if ctx.Capabilities == nil {
		ctx.Capabilities = NewLocalCapabilityRegistry()
	}
	return ctx
}

func (ctx *ServiceContext[T]) CloseCapabilities() error {
	if ctx == nil || ctx.Capabilities == nil {
		return nil
	}
	return ctx.Capabilities.Close()
}

func (ctx *ServiceContext[T]) CapabilityLocalFirst(name string) (any, bool) {
	if ctx == nil || ctx.Capabilities == nil {
		return nil, false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false
	}
	if ctx.Name != "" && !strings.HasPrefix(name, ctx.Name+".") {
		if value, ok := ctx.Capabilities.Get(ctx.Name + "." + name); ok {
			return value, true
		}
	}
	return ctx.Capabilities.Get(name)
}
