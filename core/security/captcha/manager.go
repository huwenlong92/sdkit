package captcha

import "context"

type Manager struct {
	provider Provider
}

func NewManager(provider Provider) *Manager {
	if provider == nil {
		provider = NoopProvider{}
	}
	return &Manager{provider: provider}
}

func (m *Manager) Verify(ctx context.Context, token string, ip string) error {
	return m.provider.Verify(ctx, token, ip)
}

func (m *Manager) ProviderName() string {
	return m.provider.Name()
}
