package runtime

const (
	GroupAPI      = "api"
	GroupWorker   = "worker"
	GroupSystem   = "system"
	GroupInternal = "internal"
)

const (
	ScopeGlobal       = "global"
	ScopeServiceLocal = "service_local"
)

type CapabilityMetadata struct {
	Name        string
	Description string
	Group       string
	Scope       string
	Internal    bool
}

type ProviderMetadata struct {
	Name        string
	Description string
	Group       string
	Internal    bool
	Mode        ProviderMode
}

type CommandMetadata struct {
	Name        string
	Description string
	Group       string
	Internal    bool
}

func capabilityMetadata(capability CapabilityContract) CapabilityMetadata {
	if capability == nil {
		return CapabilityMetadata{}
	}
	metadata := capability.Metadata()
	if metadata.Name == "" {
		metadata.Name = capability.Name()
	}
	metadata.Scope = normalizeCapabilityScope(metadata.Scope)
	return metadata
}

func capabilityName(capability CapabilityContract) string {
	return capabilityMetadata(capability).Name
}

func capabilityDependencies(capability CapabilityContract) []Dependency {
	if capability == nil {
		return nil
	}
	return cloneDependencies(capability.Dependencies())
}

func normalizeCapabilityScope(scope string) string {
	if scope == "" {
		return ScopeGlobal
	}
	return scope
}

func providerMetadata(provider ProviderContract) ProviderMetadata {
	if provider == nil {
		return ProviderMetadata{}
	}
	metadata := provider.Metadata()
	if metadata.Name == "" {
		metadata.Name = provider.Name()
	}
	metadata.Mode = providerMode(provider)
	return metadata
}

func providerName(provider ProviderContract) string {
	return providerMetadata(provider).Name
}

func ProviderModeOf(provider ProviderContract) ProviderMode {
	return providerMode(provider)
}

func providerMode(provider ProviderContract) ProviderMode {
	if provider == nil {
		return ProviderModeJob
	}
	if modeProvider, ok := provider.(interface{ ProviderMode() ProviderMode }); ok {
		return normalizeProviderMode(modeProvider.ProviderMode())
	}
	return normalizeProviderMode(provider.Metadata().Mode)
}

func normalizeProviderMode(mode ProviderMode) ProviderMode {
	switch mode {
	case ProviderModeService:
		return ProviderModeService
	case ProviderModeJob, "":
		return ProviderModeJob
	default:
		return ProviderModeJob
	}
}

func providerDependencies(provider ProviderContract) []Dependency {
	if provider == nil {
		return nil
	}
	return cloneDependencies(provider.Dependencies())
}

func commandMetadata(command Command) CommandMetadata {
	if command == nil {
		return CommandMetadata{}
	}
	metadata := command.Metadata()
	if metadata.Name == "" {
		metadata.Name = command.Name()
	}
	return metadata
}
