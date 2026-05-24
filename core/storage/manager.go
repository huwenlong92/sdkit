package storage

import (
	"fmt"
	"strings"
	"sync"

	pkgfs "github.com/huwenlong92/sdkit/pkg/storage"
	fscore "github.com/huwenlong92/sdkit/pkg/storage/core"
)

type Manager struct {
	mu          sync.RWMutex
	defaultName string
	configs     map[string]StoreConfig
	stores      map[string]*FileSystem
}

func NewManager(cfg Config) (*Manager, error) {
	defaultName, configs := cfg.storeConfigs()
	if len(configs) == 0 {
		return nil, ErrNotConfigured
	}
	if defaultName == "" {
		return nil, ErrDefaultRequired
	}
	if _, ok := configs[defaultName]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrStoreNotFound, defaultName)
	}
	manager := &Manager{
		defaultName: defaultName,
		configs:     configs,
		stores:      make(map[string]*FileSystem, len(configs)),
	}
	if _, err := manager.Default(); err != nil {
		_ = manager.Close()
		return nil, err
	}
	return manager, nil
}

func (m *Manager) Default() (*FileSystem, error) {
	if m == nil {
		return nil, ErrNotConfigured
	}
	return m.Use(m.defaultName)
}

func (m *Manager) Use(name string) (*FileSystem, error) {
	if m == nil {
		return nil, ErrNotConfigured
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = m.defaultName
	}

	m.mu.RLock()
	if fs := m.stores[name]; fs != nil {
		m.mu.RUnlock()
		return fs, nil
	}
	cfg, ok := m.configs[name]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrStoreNotFound, name)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if fs := m.stores[name]; fs != nil {
		return fs, nil
	}
	nextCfg := cfg.storageConfig()
	nextCfg.StoreName = name
	nextCfg.DefaultStore = name == m.defaultName
	fs, err := pkgfs.New(&nextCfg)
	if err != nil {
		return nil, err
	}
	m.stores[name] = fs
	return fs, nil
}

func (m *Manager) New(policy Policy, opts ...Option) (*FileSystem, error) {
	return pkgfs.NewFromPolicy(policy, opts...)
}

func (m *Manager) Policy(name string) (Policy, error) {
	if m == nil {
		return Policy{}, ErrNotConfigured
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = m.defaultName
	}
	m.mu.RLock()
	cfg, ok := m.configs[name]
	m.mu.RUnlock()
	if !ok {
		return Policy{}, fmt.Errorf("%w: %s", ErrStoreNotFound, name)
	}
	return cfg.storageConfig().Policy, nil
}

func (m *Manager) Config(name string) (StoreConfig, error) {
	if m == nil {
		return StoreConfig{}, ErrNotConfigured
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = m.defaultName
	}
	m.mu.RLock()
	cfg, ok := m.configs[name]
	m.mu.RUnlock()
	if !ok {
		return StoreConfig{}, fmt.Errorf("%w: %s", ErrStoreNotFound, name)
	}
	return cfg.Clone(), nil
}

func (m *Manager) AccessPath(name string, objectPath string) string {
	objectPath = strings.TrimSpace(objectPath)
	if objectPath == "" || hasURLScheme(objectPath) {
		return objectPath
	}
	if m == nil {
		return objectPath
	}
	name = strings.TrimSpace(name)

	m.mu.RLock()
	defaultName := m.defaultName
	if name == "" {
		name = defaultName
	}
	if name == "" || name == defaultName {
		m.mu.RUnlock()
		return objectPath
	}
	cfg, ok := m.configs[name]
	m.mu.RUnlock()
	if !ok {
		return objectPath
	}
	baseURL := strings.TrimSpace(cfg.storageConfig().Policy.CDNURL)
	if baseURL == "" {
		return objectPath
	}
	if accessURL := fscore.JoinObjectURL(baseURL, objectPath); accessURL != "" {
		return accessURL
	}
	return objectPath
}

func (m *Manager) DefaultName() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultName
}

func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	stores := m.stores
	m.stores = make(map[string]*FileSystem)
	m.mu.Unlock()

	var closeErr error
	for _, fs := range stores {
		if err := fs.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func New(policy Policy, opts ...Option) (*FileSystem, error) {
	return pkgfs.NewFromPolicy(policy, opts...)
}

func NewFileSystem(policy Policy, opts ...Option) (*FileSystem, error) {
	return pkgfs.NewFileSystem(policy, opts...)
}

func NewFromPolicy(policy fscore.StoragePolicy, opts ...pkgfs.Option) (*FileSystem, error) {
	return pkgfs.NewFromPolicy(policy, opts...)
}

func hasURLScheme(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}
