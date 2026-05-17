package runtime

import (
	"errors"
	"fmt"
)

var (
	ErrDependencyNameRequired = errors.New("runtime: dependency name is required")
	ErrDependencyMissing      = errors.New("runtime: dependency is missing")
	ErrDependencyDuplicate    = errors.New("runtime: dependency is duplicate")
	ErrDependencyCycle        = errors.New("runtime: dependency cycle detected")
)

type Dependency struct {
	Name     string
	Required bool
}

type DependencyMetadata struct {
	Source   string
	Target   string
	Required bool
}

func Require(name string) Dependency {
	return Dependency{Name: name, Required: true}
}

func Optional(name string) Dependency {
	return Dependency{Name: name}
}

func RequireCapabilities(names ...string) []Dependency {
	return dependenciesFromNames(true, names...)
}

func OptionalCapabilities(names ...string) []Dependency {
	return dependenciesFromNames(false, names...)
}

func (a *App) Dependencies() []DependencyMetadata {
	if a == nil {
		return nil
	}
	capabilities, err := a.capabilitiesWithProviderDeclarations()
	if err != nil {
		capabilities = a.Capabilities()
	}
	return dependencyMetadata(capabilities, a.Providers())
}

func (a *App) ValidateDependencies() error {
	if a == nil {
		return ErrAppNil
	}
	capabilities, err := a.capabilitiesWithProviderDeclarations()
	if err != nil {
		return err
	}
	return ValidateDependencies(capabilities, a.Providers())
}

func ValidateDependencies(capabilities []Capability, providers []Provider) error {
	if err := validateCapabilityDependencies(capabilities); err != nil {
		return err
	}
	return validateProviderDependencies(providers, capabilities)
}

func SortCapabilities(capabilities []Capability) ([]Capability, error) {
	if err := validateCapabilityDependencies(capabilities); err != nil {
		return nil, err
	}
	items := make([]dependencyItem[Capability], 0, len(capabilities))
	for _, capability := range capabilities {
		items = append(items, dependencyItem[Capability]{
			name:         capabilityName(capability),
			value:        capability,
			dependencies: capabilityDependencies(capability),
		})
	}
	return sortDependencyItems(items)
}

func SortProviders(providers []Provider, capabilities []Capability) ([]Provider, error) {
	if err := validateProviderDependencies(providers, capabilities); err != nil {
		return nil, err
	}
	providerNames := makeNameSet(providers, func(provider Provider) string {
		return providerName(provider)
	})
	items := make([]dependencyItem[Provider], 0, len(providers))
	for _, provider := range providers {
		dependencies := filterDependencies(providerDependencies(provider), func(dep Dependency) bool {
			_, ok := providerNames[dep.Name]
			return ok
		})
		items = append(items, dependencyItem[Provider]{
			name:         providerName(provider),
			value:        provider,
			dependencies: dependencies,
		})
	}
	return sortDependencyItems(items)
}

func selectProvidersForRun(name string, providers []Provider, capabilities []Capability, extraCapabilityNames map[string]struct{}) ([]Provider, error) {
	providerByName := make(map[string]Provider, len(providers))
	for _, provider := range providers {
		providerByName[providerName(provider)] = provider
	}
	if _, ok := providerByName[name]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, name)
	}

	capabilityNames := makeNameSet(capabilities, func(capability Capability) string {
		return capabilityName(capability)
	})
	for capabilityName := range extraCapabilityNames {
		capabilityNames[capabilityName] = struct{}{}
	}
	targetNames := cloneNameSet(capabilityNames)
	for providerName := range providerByName {
		targetNames[providerName] = struct{}{}
	}

	selected := make(map[string]struct{})
	visiting := make(map[string]struct{})
	visited := make(map[string]struct{})
	var visit func(string) error
	visit = func(current string) error {
		if _, ok := visited[current]; ok {
			return nil
		}
		if _, ok := visiting[current]; ok {
			return ErrDependencyCycle
		}
		provider := providerByName[current]
		visiting[current] = struct{}{}
		if err := validateDependencyList(current, providerDependencies(provider), targetNames); err != nil {
			return err
		}
		for _, dep := range providerDependencies(provider) {
			if _, ok := providerByName[dep.Name]; !ok {
				continue
			}
			if err := visit(dep.Name); err != nil {
				return err
			}
		}
		delete(visiting, current)
		visited[current] = struct{}{}
		selected[current] = struct{}{}
		return nil
	}
	if err := visit(name); err != nil {
		return nil, err
	}

	out := make([]Provider, 0, len(selected))
	for _, provider := range providers {
		if _, ok := selected[providerName(provider)]; ok {
			out = append(out, provider)
		}
	}
	selectedProviderNames := makeNameSet(out, func(provider Provider) string {
		return providerName(provider)
	})
	return sortProvidersNoValidate(out, selectedProviderNames)
}

func dependencyMetadata(capabilities []Capability, providers []Provider) []DependencyMetadata {
	out := make([]DependencyMetadata, 0)
	for _, capability := range capabilities {
		source := capabilityName(capability)
		for _, dep := range capabilityDependencies(capability) {
			out = append(out, DependencyMetadata{Source: source, Target: dep.Name, Required: dep.Required})
		}
	}
	for _, provider := range providers {
		source := providerName(provider)
		for _, dep := range providerDependencies(provider) {
			out = append(out, DependencyMetadata{Source: source, Target: dep.Name, Required: dep.Required})
		}
	}
	return out
}

func validateCapabilityDependencies(capabilities []Capability) error {
	names := makeNameSet(capabilities, func(capability Capability) string {
		return capabilityName(capability)
	})
	for _, capability := range capabilities {
		source := capabilityName(capability)
		if err := validateDependencyList(source, capabilityDependencies(capability), names); err != nil {
			return err
		}
	}
	_, err := sortCapabilitiesNoValidate(capabilities)
	return err
}

func validateProviderDependencies(providers []Provider, capabilities []Capability) error {
	providerNames := makeNameSet(providers, func(provider Provider) string {
		return providerName(provider)
	})
	capabilityNames := makeNameSet(capabilities, func(capability Capability) string {
		return capabilityName(capability)
	})
	targetNames := cloneNameSet(providerNames)
	for name := range capabilityNames {
		targetNames[name] = struct{}{}
	}

	for _, provider := range providers {
		source := providerName(provider)
		if err := validateDependencyList(source, providerDependencies(provider), targetNames); err != nil {
			return err
		}
	}
	_, err := sortProvidersNoValidate(providers, providerNames)
	return err
}

func validateDependencyList(source string, dependencies []Dependency, targetNames map[string]struct{}) error {
	seen := make(map[string]struct{}, len(dependencies))
	for _, dep := range dependencies {
		if dep.Name == "" {
			return fmt.Errorf("%w: %s", ErrDependencyNameRequired, source)
		}
		if _, ok := seen[dep.Name]; ok {
			return fmt.Errorf("%w: %s -> %s", ErrDependencyDuplicate, source, dep.Name)
		}
		seen[dep.Name] = struct{}{}
		if !dep.Required {
			continue
		}
		if _, ok := targetNames[dep.Name]; !ok {
			return fmt.Errorf("%w: %s -> %s", ErrDependencyMissing, source, dep.Name)
		}
	}
	return nil
}

func sortCapabilitiesNoValidate(capabilities []Capability) ([]Capability, error) {
	items := make([]dependencyItem[Capability], 0, len(capabilities))
	for _, capability := range capabilities {
		items = append(items, dependencyItem[Capability]{
			name:         capabilityName(capability),
			value:        capability,
			dependencies: capabilityDependencies(capability),
		})
	}
	return sortDependencyItems(items)
}

func sortProvidersNoValidate(providers []Provider, providerNames map[string]struct{}) ([]Provider, error) {
	items := make([]dependencyItem[Provider], 0, len(providers))
	for _, provider := range providers {
		dependencies := filterDependencies(providerDependencies(provider), func(dep Dependency) bool {
			_, ok := providerNames[dep.Name]
			return ok
		})
		items = append(items, dependencyItem[Provider]{
			name:         providerName(provider),
			value:        provider,
			dependencies: dependencies,
		})
	}
	return sortDependencyItems(items)
}

type dependencyItem[T any] struct {
	name         string
	value        T
	dependencies []Dependency
}

func sortDependencyItems[T any](items []dependencyItem[T]) ([]T, error) {
	if len(items) == 0 {
		return nil, nil
	}

	indexByName := make(map[string]int, len(items))
	for i, item := range items {
		indexByName[item.name] = i
	}

	inDegree := make([]int, len(items))
	edges := make([][]int, len(items))
	for i, item := range items {
		for _, dep := range item.dependencies {
			dependencyIndex, ok := indexByName[dep.Name]
			if !ok {
				continue
			}
			edges[dependencyIndex] = append(edges[dependencyIndex], i)
			inDegree[i]++
		}
	}

	queue := make([]int, 0, len(items))
	for i := range items {
		if inDegree[i] == 0 {
			queue = append(queue, i)
		}
	}

	out := make([]T, 0, len(items))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		out = append(out, items[current].value)
		for _, next := range edges[current] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if len(out) != len(items) {
		return nil, ErrDependencyCycle
	}
	return out, nil
}

func makeNameSet[T any](items []T, name func(T) string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		out[name(item)] = struct{}{}
	}
	return out
}

func cloneNameSet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for name := range in {
		out[name] = struct{}{}
	}
	return out
}

func filterDependencies(dependencies []Dependency, keep func(Dependency) bool) []Dependency {
	if len(dependencies) == 0 {
		return nil
	}
	out := make([]Dependency, 0, len(dependencies))
	for _, dep := range dependencies {
		if keep(dep) {
			out = append(out, dep)
		}
	}
	return out
}

func cloneDependencies(dependencies []Dependency) []Dependency {
	if len(dependencies) == 0 {
		return nil
	}
	out := make([]Dependency, len(dependencies))
	copy(out, dependencies)
	return out
}

func dependenciesFromNames(required bool, names ...string) []Dependency {
	if len(names) == 0 {
		return nil
	}
	out := make([]Dependency, 0, len(names))
	for _, name := range names {
		out = append(out, Dependency{Name: name, Required: required})
	}
	return out
}
