package runtime

import (
	"errors"
	"fmt"
	"sync"
)

var ErrRegistryNil = errors.New("runtime: registry is nil")

type Registry struct {
	mu sync.RWMutex

	capabilities       []Capability
	capabilityByName   map[string]Capability
	capabilityMetadata map[string]CapabilityMetadata

	providers        []Provider
	providerByName   map[string]Provider
	providerMetadata map[string]ProviderMetadata

	commands        []Command
	commandByName   map[string]Command
	commandMetadata map[string]CommandMetadata

	adapters        []Adapter
	adapterByName   map[string]Adapter
	adapterMetadata map[string]AdapterMetadata

	plugins        []Plugin
	pluginByName   map[string]Plugin
	pluginMetadata map[string]PluginMetadata
}

func NewRegistry() *Registry {
	return &Registry{
		capabilityByName:   make(map[string]Capability),
		capabilityMetadata: make(map[string]CapabilityMetadata),
		providerByName:     make(map[string]Provider),
		providerMetadata:   make(map[string]ProviderMetadata),
		commandByName:      make(map[string]Command),
		commandMetadata:    make(map[string]CommandMetadata),
		adapterByName:      make(map[string]Adapter),
		adapterMetadata:    make(map[string]AdapterMetadata),
		pluginByName:       make(map[string]Plugin),
		pluginMetadata:     make(map[string]PluginMetadata),
	}
}

func (r *Registry) RegisterCapabilities(capabilities ...CapabilityContract) error {
	if r == nil {
		return ErrRegistryNil
	}
	accepted := make([]CapabilityContract, 0, len(capabilities))
	metadataByName := make(map[string]CapabilityMetadata, len(capabilities))

	r.mu.Lock()
	defer r.mu.Unlock()

	existing := make(map[string]struct{}, len(r.capabilityByName)+len(capabilities))
	for name := range r.capabilityByName {
		existing[name] = struct{}{}
	}

	for _, capability := range capabilities {
		if capability == nil {
			return ErrCapabilityNil
		}
		metadata := capabilityMetadata(capability)
		if metadata.Name == "" {
			return ErrCapabilityNameRequired
		}
		if isReservedCapabilityName(metadata.Name) {
			return fmt.Errorf("%w: %s", ErrCapabilityNameReserved, metadata.Name)
		}
		if _, ok := existing[metadata.Name]; ok {
			return fmt.Errorf("%w: %s", ErrCapabilityNameDuplicate, metadata.Name)
		}
		existing[metadata.Name] = struct{}{}
		accepted = append(accepted, capability)
		metadataByName[metadata.Name] = metadata
	}

	for _, capability := range accepted {
		metadata := metadataByName[capabilityMetadata(capability).Name]
		runtimeCapability := newRuntimeCapability(capability)
		r.capabilities = append(r.capabilities, runtimeCapability)
		r.capabilityByName[metadata.Name] = runtimeCapability
		r.capabilityMetadata[metadata.Name] = metadata
	}
	return nil
}

func (r *Registry) Capability(name string) (Capability, bool) {
	if r == nil || name == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	capability, ok := r.capabilityByName[name]
	return capability, ok
}

func (r *Registry) Capabilities() []Capability {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.capabilities) == 0 {
		return nil
	}
	out := make([]Capability, len(r.capabilities))
	copy(out, r.capabilities)
	return out
}

func (r *Registry) CapabilitiesByGroup(group string) []Capability {
	if r == nil || group == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Capability, 0)
	for _, capability := range r.capabilities {
		metadata := r.capabilityMetadata[capabilityMetadata(capability).Name]
		if metadata.Group == group {
			out = append(out, capability)
		}
	}
	return out
}

func (r *Registry) CapabilitiesByScope(scope string) []Capability {
	if r == nil {
		return nil
	}
	scope = normalizeCapabilityScope(scope)
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Capability, 0)
	for _, capability := range r.capabilities {
		metadata := r.capabilityMetadata[capabilityMetadata(capability).Name]
		if normalizeCapabilityScope(metadata.Scope) == scope {
			out = append(out, capability)
		}
	}
	return out
}

func (r *Registry) RegisterProviders(providers ...ProviderContract) error {
	if r == nil {
		return ErrRegistryNil
	}
	accepted := make([]ProviderContract, 0, len(providers))
	metadataByName := make(map[string]ProviderMetadata, len(providers))

	r.mu.Lock()
	defer r.mu.Unlock()

	existing := make(map[string]struct{}, len(r.providerByName)+len(providers))
	for name := range r.providerByName {
		existing[name] = struct{}{}
	}

	for _, provider := range providers {
		if provider == nil {
			return ErrProviderNil
		}
		metadata := providerMetadata(provider)
		if metadata.Name == "" {
			return ErrProviderNameRequired
		}
		if isReservedProviderName(metadata.Name) {
			return fmt.Errorf("%w: %s", ErrProviderNameReserved, metadata.Name)
		}
		if _, ok := existing[metadata.Name]; ok {
			return fmt.Errorf("%w: %s", ErrProviderNameDuplicate, metadata.Name)
		}
		existing[metadata.Name] = struct{}{}
		accepted = append(accepted, provider)
		metadataByName[metadata.Name] = metadata
	}

	for _, provider := range accepted {
		metadata := metadataByName[providerMetadata(provider).Name]
		runtimeProvider := newRuntimeProvider(provider)
		r.providers = append(r.providers, runtimeProvider)
		r.providerByName[metadata.Name] = runtimeProvider
		r.providerMetadata[metadata.Name] = metadata
	}
	return nil
}

func (r *Registry) Provider(name string) (Provider, bool) {
	if r == nil || name == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providerByName[name]
	return provider, ok
}

func (r *Registry) Providers() []Provider {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.providers) == 0 {
		return nil
	}
	out := make([]Provider, len(r.providers))
	copy(out, r.providers)
	return out
}

func (r *Registry) ProvidersByGroup(group string) []Provider {
	if r == nil || group == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Provider, 0)
	for _, provider := range r.providers {
		metadata := r.providerMetadata[providerMetadata(provider).Name]
		if metadata.Group == group {
			out = append(out, provider)
		}
	}
	return out
}

func (r *Registry) RegisterCommands(commands ...Command) error {
	if r == nil {
		return ErrRegistryNil
	}
	accepted := make([]Command, 0, len(commands))
	metadataByName := make(map[string]CommandMetadata, len(commands))

	r.mu.Lock()
	defer r.mu.Unlock()

	existing := make(map[string]struct{}, len(r.commandByName)+len(commands))
	for name := range r.commandByName {
		existing[name] = struct{}{}
	}

	for _, command := range commands {
		if command == nil {
			return ErrCommandNil
		}
		metadata := commandMetadata(command)
		if metadata.Name == "" {
			return ErrCommandNameRequired
		}
		if isReservedCommandName(metadata.Name) {
			return fmt.Errorf("%w: %s", ErrCommandNameReserved, metadata.Name)
		}
		if _, ok := existing[metadata.Name]; ok {
			return fmt.Errorf("%w: %s", ErrCommandNameDuplicate, metadata.Name)
		}
		existing[metadata.Name] = struct{}{}
		accepted = append(accepted, command)
		metadataByName[metadata.Name] = metadata
	}

	for _, command := range accepted {
		metadata := metadataByName[commandMetadata(command).Name]
		r.commands = append(r.commands, command)
		r.commandByName[metadata.Name] = command
		r.commandMetadata[metadata.Name] = metadata
	}
	return nil
}

func (r *Registry) Command(name string) (Command, bool) {
	if r == nil || name == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	command, ok := r.commandByName[name]
	return command, ok
}

func (r *Registry) Commands() []Command {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.commands) == 0 {
		return nil
	}
	out := make([]Command, len(r.commands))
	copy(out, r.commands)
	return out
}

func (r *Registry) CommandsByGroup(group string) []Command {
	if r == nil || group == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Command, 0)
	for _, command := range r.commands {
		metadata := r.commandMetadata[commandMetadata(command).Name]
		if metadata.Group == group {
			out = append(out, command)
		}
	}
	return out
}

func (r *Registry) RegisterAdapters(adapters ...Adapter) error {
	if r == nil {
		return ErrRegistryNil
	}
	accepted := make([]Adapter, 0, len(adapters))
	metadataByName := make(map[string]AdapterMetadata, len(adapters))

	r.mu.Lock()
	defer r.mu.Unlock()

	existing := make(map[string]struct{}, len(r.adapterByName)+len(adapters))
	for name := range r.adapterByName {
		existing[name] = struct{}{}
	}

	for _, adapter := range adapters {
		if adapter == nil {
			return ErrAdapterNil
		}
		metadata := adapterMetadata(adapter)
		if metadata.Name == "" {
			return ErrAdapterNameRequired
		}
		if metadata.Type == "" {
			return ErrAdapterTypeRequired
		}
		if !isSupportedAdapterType(metadata.Type) {
			return fmt.Errorf("%w: %s", ErrAdapterTypeUnsupported, metadata.Type)
		}
		if _, ok := existing[metadata.Name]; ok {
			return fmt.Errorf("%w: %s", ErrAdapterNameDuplicate, metadata.Name)
		}
		existing[metadata.Name] = struct{}{}
		accepted = append(accepted, adapter)
		metadataByName[metadata.Name] = metadata
	}

	for _, adapter := range accepted {
		metadata := metadataByName[adapterMetadata(adapter).Name]
		r.adapters = append(r.adapters, adapter)
		r.adapterByName[metadata.Name] = adapter
		r.adapterMetadata[metadata.Name] = metadata
	}
	return nil
}

func (r *Registry) Adapter(name string) (Adapter, bool) {
	if r == nil || name == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapterByName[name]
	return adapter, ok
}

func (r *Registry) Adapters() []Adapter {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.adapters) == 0 {
		return nil
	}
	out := make([]Adapter, len(r.adapters))
	copy(out, r.adapters)
	return out
}

func (r *Registry) AdaptersByType(adapterType string) []Adapter {
	if r == nil || adapterType == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Adapter, 0)
	for _, adapter := range r.adapters {
		metadata := r.adapterMetadata[adapterMetadata(adapter).Name]
		if metadata.Type == adapterType {
			out = append(out, adapter)
		}
	}
	return out
}

func (r *Registry) RegisterPlugins(plugins ...Plugin) error {
	if r == nil {
		return ErrRegistryNil
	}
	accepted := make([]Plugin, 0, len(plugins))
	metadataByName := make(map[string]PluginMetadata, len(plugins))

	r.mu.Lock()
	defer r.mu.Unlock()

	existing := make(map[string]struct{}, len(r.pluginByName)+len(plugins))
	for name := range r.pluginByName {
		existing[name] = struct{}{}
	}

	for _, plugin := range plugins {
		if plugin == nil {
			return ErrPluginNil
		}
		metadata := pluginMetadata(plugin)
		if metadata.Name == "" {
			return ErrPluginNameRequired
		}
		if isReservedPluginName(metadata.Name) {
			return fmt.Errorf("%w: %s", ErrPluginNameReserved, metadata.Name)
		}
		if _, ok := existing[metadata.Name]; ok {
			return fmt.Errorf("%w: %s", ErrPluginNameDuplicate, metadata.Name)
		}
		existing[metadata.Name] = struct{}{}
		accepted = append(accepted, plugin)
		metadataByName[metadata.Name] = metadata
	}

	for _, plugin := range accepted {
		metadata := metadataByName[pluginMetadata(plugin).Name]
		r.plugins = append(r.plugins, plugin)
		r.pluginByName[metadata.Name] = plugin
		r.pluginMetadata[metadata.Name] = metadata
	}
	return nil
}

func (r *Registry) Plugin(name string) (Plugin, bool) {
	if r == nil || name == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	plugin, ok := r.pluginByName[name]
	return plugin, ok
}

func (r *Registry) Plugins() []Plugin {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.plugins) == 0 {
		return nil
	}
	out := make([]Plugin, len(r.plugins))
	copy(out, r.plugins)
	return out
}

func (r *Registry) PluginsByGroup(group string) []Plugin {
	if r == nil || group == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Plugin, 0)
	for _, plugin := range r.plugins {
		metadata := r.pluginMetadata[pluginMetadata(plugin).Name]
		if metadata.Group == group {
			out = append(out, plugin)
		}
	}
	return out
}
