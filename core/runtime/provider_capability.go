package runtime

import "fmt"

func (a *App) registerProviderCapabilities(providers []Provider) error {
	capabilities, err := a.collectProviderCapabilities(providers)
	if err != nil {
		return err
	}
	if len(capabilities) == 0 {
		return nil
	}
	if err := a.RegisterCapabilities(capabilities...); err != nil {
		return err
	}
	a.markProviderCapabilities(capabilities)
	return nil
}

func (a *App) collectProviderCapabilities(providers []Provider) ([]CapabilityContract, error) {
	if a == nil || len(providers) == 0 {
		return nil, nil
	}
	out := make([]CapabilityContract, 0)
	for _, provider := range providers {
		for _, capability := range providerRuntimeCapabilities(provider) {
			if capability == nil {
				return nil, ErrCapabilityNil
			}
			name := capabilityName(capability)
			if name != "" && a.providerCapabilityRegistered(name) {
				continue
			}
			out = append(out, capability)
		}
	}
	return out, nil
}

func (a *App) capabilitiesWithProviderDeclarations() ([]Capability, error) {
	if a == nil {
		return nil, ErrAppNil
	}
	capabilities := a.Capabilities()
	existing := makeNameSet(capabilities, func(capability Capability) string {
		return capabilityName(capability)
	})
	declared := make(map[string]struct{})
	for _, provider := range a.Providers() {
		for _, capability := range providerRuntimeCapabilities(provider) {
			if capability == nil {
				return nil, ErrCapabilityNil
			}
			metadata := capabilityMetadata(capability)
			if metadata.Name == "" {
				return nil, ErrCapabilityNameRequired
			}
			if isReservedCapabilityName(metadata.Name) {
				return nil, fmt.Errorf("%w: %s", ErrCapabilityNameReserved, metadata.Name)
			}
			if _, ok := existing[metadata.Name]; ok {
				if a.providerCapabilityRegistered(metadata.Name) {
					continue
				}
				return nil, fmt.Errorf("%w: %s", ErrCapabilityNameDuplicate, metadata.Name)
			}
			if _, ok := declared[metadata.Name]; ok {
				return nil, fmt.Errorf("%w: %s", ErrCapabilityNameDuplicate, metadata.Name)
			}
			declared[metadata.Name] = struct{}{}
			capabilities = append(capabilities, newRuntimeCapability(capability))
		}
	}
	return capabilities, nil
}

func providerRuntimeCapabilityNameSet(providers []Provider) map[string]struct{} {
	out := make(map[string]struct{})
	for _, provider := range providers {
		for _, capability := range providerRuntimeCapabilities(provider) {
			if capability == nil {
				continue
			}
			name := capabilityName(capability)
			if name == "" {
				continue
			}
			out[name] = struct{}{}
		}
	}
	return out
}

func (a *App) providerCapabilityRegistered(name string) bool {
	if a == nil || name == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.providerCapabilityNames[name]
	return ok
}

func (a *App) markProviderCapabilities(capabilities []CapabilityContract) {
	if a == nil || len(capabilities) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.providerCapabilityNames == nil {
		a.providerCapabilityNames = make(map[string]struct{}, len(capabilities))
	}
	for _, capability := range capabilities {
		name := capabilityName(capability)
		if name == "" {
			continue
		}
		a.providerCapabilityNames[name] = struct{}{}
	}
}
