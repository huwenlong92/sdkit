package payment

import (
	"fmt"
	"sync"
)

type Registry struct {
	mu       sync.RWMutex
	adapters map[Provider]ProviderAdapter
}

func NewRegistry() *Registry {
	return &Registry{adapters: make(map[Provider]ProviderAdapter)}
}

func (r *Registry) Register(adapter ProviderAdapter) error {
	if adapter == nil {
		return fmt.Errorf("%w: nil adapter", ErrAdapterNotFound)
	}
	name := adapter.Name()
	if name == "" {
		return fmt.Errorf("%w: empty provider", ErrUnsupportedProvider)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.adapters == nil {
		r.adapters = make(map[Provider]ProviderAdapter)
	}
	if _, exists := r.adapters[name]; exists {
		return fmt.Errorf("%w: %s", ErrAdapterAlreadyExists, name)
	}
	r.adapters[name] = adapter
	return nil
}

func (r *Registry) Adapter(provider Provider) (ProviderAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapters[provider]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAdapterNotFound, provider)
	}
	return adapter, nil
}

func (r *Registry) Adapters() map[Provider]ProviderAdapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[Provider]ProviderAdapter, len(r.adapters))
	for provider, adapter := range r.adapters {
		out[provider] = adapter
	}
	return out
}
