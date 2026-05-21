package captcha

import (
	"context"
	"fmt"
)

type Manager struct {
	providers map[Kind]Provider
}

func NewManager(providers ...Provider) *Manager {
	m := &Manager{providers: make(map[Kind]Provider)}
	for _, provider := range providers {
		m.Register(provider)
	}
	if len(m.providers) == 0 {
		m.Register(NoopProvider{})
	}
	return m
}

func (m *Manager) Register(provider Provider) {
	if provider != nil {
		m.providers[provider.Kind()] = provider
	}
}

func (m *Manager) Generate(ctx context.Context, kind Kind, opts GenerateOptions) (*Challenge, error) {
	provider, ok := m.provider(kind)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, kind)
	}
	return provider.Generate(ctx, opts)
}

func (m *Manager) Verify(ctx context.Context, kind Kind, id string, answer string) error {
	provider, ok := m.provider(kind)
	if !ok {
		return fmt.Errorf("%w: %s", ErrProviderNotFound, kind)
	}
	return provider.Verify(ctx, id, answer)
}

func (m *Manager) ProviderName(kind Kind) string {
	provider, ok := m.provider(kind)
	if !ok {
		return ""
	}
	return provider.Name()
}

func (m *Manager) provider(kind Kind) (Provider, bool) {
	if m == nil {
		return nil, false
	}
	provider, ok := m.providers[kind]
	return provider, ok
}
