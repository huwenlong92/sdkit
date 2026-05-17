package runtime

import (
	"errors"
	"sync"
)

type LocalCapability interface {
	Name() string
}

type localCapabilityCloser interface {
	Close() error
}

type LocalCapabilityRegistry struct {
	mu        sync.RWMutex
	instances map[string]any
	names     []string
}

func NewLocalCapabilityRegistry() *LocalCapabilityRegistry {
	return &LocalCapabilityRegistry{}
}

func (r *LocalCapabilityRegistry) Add(capability LocalCapability) {
	if r == nil || capability == nil {
		return
	}
	r.AddName(capability.Name())
}

func (r *LocalCapabilityRegistry) AddName(name string) {
	if r == nil || name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addNameLocked(name)
}

func (r *LocalCapabilityRegistry) Set(name string, instance any) {
	if r == nil || name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addNameLocked(name)
	if instance == nil {
		return
	}
	if r.instances == nil {
		r.instances = make(map[string]any)
	}
	r.instances[name] = instance
}

func (r *LocalCapabilityRegistry) Get(name string) (any, bool) {
	if r == nil || name == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.instances[name]
	return value, ok
}

func LocalCapabilityAs[T any](registry *LocalCapabilityRegistry, name string) (T, bool) {
	var zero T
	if registry == nil {
		return zero, false
	}
	value, ok := registry.Get(name)
	if !ok {
		return zero, false
	}
	typed, ok := value.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}

func (r *LocalCapabilityRegistry) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	names := closeOrder(r.names)
	instances := r.instances
	r.instances = nil
	r.mu.Unlock()

	var errs []error
	for _, name := range names {
		closer, ok := instances[name].(localCapabilityCloser)
		if !ok || closer == nil {
			continue
		}
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *LocalCapabilityRegistry) addNameLocked(name string) {
	r.names = append(r.names, name)
}

func (r *LocalCapabilityRegistry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.names) == 0 {
		return nil
	}
	names := make([]string, 0, len(r.names))
	seen := make(map[string]struct{}, len(r.names))
	for _, name := range r.names {
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

func closeOrder(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	names := make([]string, 0, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		name := items[i]
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
