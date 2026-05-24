package sms

import (
	"context"
	"fmt"
	"strings"
	"sync"

	pkgsms "github.com/huwenlong92/sdkit/pkg/sms"
)

type Manager struct {
	mu          sync.RWMutex
	defaultName string
	configs     map[string]pkgsms.ProviderConfig
	providers   map[string]pkgsms.Provider
	middleware  []Middleware
}

func NewManager(cfg Config, middleware ...Middleware) (*Manager, error) {
	defaultName, configs := cfg.providerConfigs()
	if len(configs) == 0 {
		return nil, ErrNotConfigured
	}
	if defaultName == "" {
		return nil, ErrDefaultRequired
	}
	if _, ok := configs[defaultName]; !ok {
		return nil, providerNotFound(defaultName)
	}
	manager := &Manager{
		defaultName: defaultName,
		configs:     configs,
		providers:   make(map[string]pkgsms.Provider, len(configs)),
	}
	manager.UseMiddleware(middleware...)
	return manager, nil
}

func (m *Manager) Send(ctx context.Context, to []string, msg Message) (*SendResult, error) {
	if m == nil {
		return nil, ErrNotConfigured
	}
	var providers []string
	if providerMsg, ok := msg.(ProviderMessage); ok {
		providers = providerMsg.Providers(ctx)
	}
	if len(providers) == 0 {
		providers = []string{m.defaultName}
	}
	return m.SendVia(ctx, to, msg, providers...)
}

func (m *Manager) SendVia(ctx context.Context, to []string, msg Message, providers ...string) (*SendResult, error) {
	if m == nil {
		return nil, ErrNotConfigured
	}
	route := cleanProviderNames(providers)
	if len(route) == 0 {
		route = []string{m.defaultName}
	}
	req := Request{
		To:        append([]string(nil), to...),
		Message:   msg,
		Providers: route,
	}
	sender := Sender(SenderFunc(m.sendDirect))
	m.mu.RLock()
	middleware := append([]Middleware(nil), m.middleware...)
	m.mu.RUnlock()
	for i := len(middleware) - 1; i >= 0; i-- {
		sender = middleware[i](sender)
	}
	return sender.Send(ctx, req)
}

func (m *Manager) Use(name string) (pkgsms.Provider, error) {
	if m == nil {
		return nil, ErrNotConfigured
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = m.defaultName
	}

	m.mu.RLock()
	if provider := m.providers[name]; provider != nil {
		m.mu.RUnlock()
		return provider, nil
	}
	cfg, ok := m.configs[name]
	m.mu.RUnlock()
	if !ok {
		return nil, providerNotFound(name)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if provider := m.providers[name]; provider != nil {
		return provider, nil
	}
	provider, err := pkgsms.NewProvider(name, cfg)
	if err != nil {
		return nil, err
	}
	m.providers[name] = provider
	return provider, nil
}

func (m *Manager) Config(name string) (pkgsms.ProviderConfig, error) {
	if m == nil {
		return pkgsms.ProviderConfig{}, ErrNotConfigured
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = m.defaultName
	}
	m.mu.RLock()
	cfg, ok := m.configs[name]
	m.mu.RUnlock()
	if !ok {
		return pkgsms.ProviderConfig{}, providerNotFound(name)
	}
	return cfg.Clone(), nil
}

func (m *Manager) DefaultName() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultName
}

func (m *Manager) UseMiddleware(middleware ...Middleware) {
	if m == nil {
		return
	}
	m.mu.Lock()
	for _, item := range middleware {
		if item != nil {
			m.middleware = append(m.middleware, item)
		}
	}
	m.mu.Unlock()
}

func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	providers := m.providers
	m.providers = make(map[string]pkgsms.Provider)
	m.mu.Unlock()

	var closeErr error
	for _, provider := range providers {
		if err := provider.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func (m *Manager) sendDirect(ctx context.Context, req Request) (*SendResult, error) {
	providers := cleanProviderNames(req.Providers)
	attempts := make([]AttemptResult, 0, len(providers))
	for _, name := range providers {
		cfg, err := m.Config(name)
		if err != nil {
			attempts = append(attempts, AttemptResult{Provider: name, Error: err})
			continue
		}
		payload, err := req.Message.Resolve(ctx, cfg)
		if err != nil {
			attempts = append(attempts, AttemptResult{Provider: name, Error: err})
			continue
		}
		provider, err := m.Use(name)
		if err != nil {
			attempts = append(attempts, AttemptResult{Provider: name, Error: err})
			continue
		}
		result, err := provider.Send(ctx, ProviderRequest{
			To:       append([]string(nil), req.To...),
			Message:  req.Message,
			Payload:  payload,
			Provider: cfg,
		})
		if err == nil && result != nil && !result.Success {
			err = fmt.Errorf("%w: %s", ErrProviderFailed, result.Message)
		}
		attempt := AttemptResult{
			Provider: name,
			Result:   result,
			Error:    err,
		}
		if err == nil {
			attempt.Success = true
			attempts = append(attempts, attempt)
			return &SendResult{Provider: name, Attempts: attempts}, nil
		}
		attempts = append(attempts, attempt)
	}
	return nil, &NoProviderAvailableError{Attempts: attempts}
}
