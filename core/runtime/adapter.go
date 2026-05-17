package runtime

import "errors"

const (
	AdapterTypeCommand    = "command"
	AdapterTypeCapability = "capability"
	AdapterTypeProvider   = "provider"
)

var (
	ErrAdapterNil             = errors.New("runtime: adapter is nil")
	ErrAdapterNameRequired    = errors.New("runtime: adapter name is required")
	ErrAdapterTypeRequired    = errors.New("runtime: adapter type is required")
	ErrAdapterTypeUnsupported = errors.New("runtime: adapter type is unsupported")
	ErrAdapterNameDuplicate   = errors.New("runtime: adapter name is duplicate")
	ErrAdapterNotFound        = errors.New("runtime: adapter not found")
)

type AdapterMetadata struct {
	Name     string
	Type     string
	Internal bool
}

type Adapter interface {
	AdapterMetadata() AdapterMetadata
}

type CommandAdapter interface {
	Adapter
	Command() Command
}

type CapabilityAdapter interface {
	Adapter
	Capability() CapabilityContract
}

type ProviderAdapter interface {
	Adapter
	Provider() ProviderContract
}

func NewCommandAdapter(metadata AdapterMetadata, command Command) CommandAdapter {
	if metadata.Name == "" && command != nil {
		metadata.Name = commandMetadata(command).Name
	}
	metadata.Type = AdapterTypeCommand
	return commandAdapter{metadata: metadata, command: command}
}

func NewCapabilityAdapter(metadata AdapterMetadata, capability CapabilityContract) CapabilityAdapter {
	if metadata.Name == "" && capability != nil {
		metadata.Name = capabilityMetadata(capability).Name
	}
	metadata.Type = AdapterTypeCapability
	return capabilityAdapter{metadata: metadata, capability: capability}
}

func NewProviderAdapter(metadata AdapterMetadata, provider ProviderContract) ProviderAdapter {
	if metadata.Name == "" && provider != nil {
		metadata.Name = providerMetadata(provider).Name
	}
	metadata.Type = AdapterTypeProvider
	return providerAdapter{metadata: metadata, provider: provider}
}

type commandAdapter struct {
	metadata AdapterMetadata
	command  Command
}

func (a commandAdapter) AdapterMetadata() AdapterMetadata {
	return a.metadata
}

func (a commandAdapter) Command() Command {
	return a.command
}

type capabilityAdapter struct {
	metadata   AdapterMetadata
	capability CapabilityContract
}

func (a capabilityAdapter) AdapterMetadata() AdapterMetadata {
	return a.metadata
}

func (a capabilityAdapter) Capability() CapabilityContract {
	return a.capability
}

type providerAdapter struct {
	metadata AdapterMetadata
	provider ProviderContract
}

func (a providerAdapter) AdapterMetadata() AdapterMetadata {
	return a.metadata
}

func (a providerAdapter) Provider() ProviderContract {
	return a.provider
}

func adapterMetadata(adapter Adapter) AdapterMetadata {
	if adapter == nil {
		return AdapterMetadata{}
	}
	return adapter.AdapterMetadata()
}

func isSupportedAdapterType(adapterType string) bool {
	switch adapterType {
	case AdapterTypeCommand, AdapterTypeCapability, AdapterTypeProvider:
		return true
	default:
		return false
	}
}
