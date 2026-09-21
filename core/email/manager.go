package email

import (
	"context"
	"strings"
	"sync"

	pkgemail "github.com/huwenlong92/sdkit/pkg/email"
)

type Manager struct {
	mu          sync.RWMutex
	defaultName string
	fallback    []string
	configs     map[string]pkgemail.ProviderConfig
	providers   map[string]pkgemail.Provider
	resolver    ProviderResolver
	templates   pkgemail.TemplateRenderer
	middleware  []Middleware
}

func NewManager(cfg Config, middleware ...Middleware) (*Manager, error) {
	defaultName, fallback, configs := cfg.providerConfigs()
	if len(configs) == 0 {
		return nil, ErrNotConfigured
	}
	if defaultName == "" {
		return nil, ErrDefaultRequired
	}
	if _, ok := configs[defaultName]; !ok {
		return nil, providerNotFound(defaultName)
	}
	for _, name := range fallback {
		if _, ok := configs[name]; !ok {
			return nil, providerNotFound(name)
		}
	}
	manager := &Manager{
		defaultName: defaultName,
		fallback:    fallback,
		configs:     configs,
		providers:   make(map[string]pkgemail.Provider, len(configs)),
		templates:   pkgemail.TemplateMap(cloneTemplates(cfg.Templates)),
	}
	manager.UseMiddleware(middleware...)
	return manager, nil
}

// NewDynamicManager 创建一个由运行时 Resolver 提供账号配置的邮件管理器。
// 动态管理器不要求默认账号，调用方应通过 SendVia 明确传入账号路由。
func NewDynamicManager(resolver ProviderResolver, renderer TemplateRenderer, middleware ...Middleware) (*Manager, error) {
	if resolver == nil {
		return nil, ErrNotConfigured
	}
	manager := &Manager{
		resolver:  resolver,
		providers: make(map[string]pkgemail.Provider),
		templates: renderer,
	}
	manager.UseMiddleware(middleware...)
	return manager, nil
}

func (m *Manager) Send(ctx context.Context, msg Message) (*SendResult, error) {
	route := m.route()
	if len(route) == 0 {
		return nil, ErrDefaultRequired
	}
	return m.SendVia(ctx, msg, route...)
}

func (m *Manager) SendVia(ctx context.Context, msg Message, providers ...string) (*SendResult, error) {
	if m == nil {
		return nil, ErrNotConfigured
	}
	route := cleanProviderNames(providers)
	if len(route) == 0 {
		route = m.route()
	}
	if len(route) == 0 {
		return nil, ErrDefaultRequired
	}
	req := Request{
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

func (m *Manager) Use(name string) (pkgemail.Provider, error) {
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
	provider, err := pkgemail.NewProvider(name, cfg)
	if err != nil {
		return nil, err
	}
	m.providers[name] = provider
	return provider, nil
}

func (m *Manager) Config(name string) (pkgemail.ProviderConfig, error) {
	if m == nil {
		return pkgemail.ProviderConfig{}, ErrNotConfigured
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = m.defaultName
	}
	m.mu.RLock()
	cfg, ok := m.configs[name]
	m.mu.RUnlock()
	if !ok {
		return pkgemail.ProviderConfig{}, providerNotFound(name)
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
	m.providers = make(map[string]pkgemail.Provider)
	m.mu.Unlock()

	var closeErr error
	for _, provider := range providers {
		if err := provider.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func (m *Manager) TemplateRenderer() pkgemail.TemplateRenderer {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.templates
}

func (m *Manager) route() []string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	route := make([]string, 0, 1+len(m.fallback))
	if m.defaultName != "" {
		route = append(route, m.defaultName)
	}
	for _, name := range m.fallback {
		if name != m.defaultName {
			route = append(route, name)
		}
	}
	return route
}

func (m *Manager) sendDirect(ctx context.Context, req Request) (*SendResult, error) {
	providers := cleanProviderNames(req.Providers)
	attempts := make([]AttemptResult, 0, len(providers))
	if req.Message == nil {
		return nil, ErrMessageRequired
	}
	m.mu.RLock()
	templates := m.templates
	m.mu.RUnlock()
	payload, err := req.Message.Resolve(ctx, templates)
	if err != nil {
		return nil, err
	}
	for _, name := range providers {
		provider, dynamic, err := m.provider(ctx, name)
		if err != nil {
			attempts = append(attempts, AttemptResult{Provider: name, Error: err})
			continue
		}
		result, err := provider.Send(ctx, payload)
		if dynamic {
			if closeErr := provider.Close(); err == nil && closeErr != nil {
				err = closeErr
			}
		}
		attempt := AttemptResult{
			Provider: name,
			Result:   result,
			Error:    err,
		}
		if err == nil {
			attempt.Success = true
			attempts = append(attempts, attempt)
			return &SendResult{Provider: name, Result: result, Attempts: attempts}, nil
		}
		attempts = append(attempts, attempt)
	}
	sendErr := &NoProviderAvailableError{Attempts: attempts}
	return &SendResult{Error: sendErr, Attempts: attempts}, sendErr
}

func (m *Manager) provider(ctx context.Context, name string) (pkgemail.Provider, bool, error) {
	m.mu.RLock()
	resolver := m.resolver
	m.mu.RUnlock()
	if resolver == nil {
		provider, err := m.Use(name)
		return provider, false, err
	}
	cfg, err := resolver.Resolve(ctx, name)
	if err != nil {
		return nil, false, err
	}
	provider, err := pkgemail.NewProvider(name, cfg)
	if err != nil {
		return nil, false, err
	}
	return provider, true, nil
}
