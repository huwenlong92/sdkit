package storage

import "sync"

var (
	defaultMu      sync.RWMutex
	defaultManager *Manager
)

func SetDefault(manager *Manager) {
	defaultMu.Lock()
	old := defaultManager
	defaultManager = manager
	defaultMu.Unlock()
	if old != nil && old != manager {
		_ = old.Close()
	}
}

func ManagerDefault() (*Manager, error) {
	defaultMu.RLock()
	manager := defaultManager
	defaultMu.RUnlock()
	if manager == nil {
		return nil, ErrNotConfigured
	}
	return manager, nil
}

func Default() (*FileSystem, error) {
	manager, err := ManagerDefault()
	if err != nil {
		return nil, err
	}
	return manager.Default()
}

func Use(name string) (*FileSystem, error) {
	manager, err := ManagerDefault()
	if err != nil {
		return nil, err
	}
	return manager.Use(name)
}

func PolicyOf(name string) (Policy, error) {
	manager, err := ManagerDefault()
	if err != nil {
		return Policy{}, err
	}
	return manager.Policy(name)
}

func Close() error {
	defaultMu.Lock()
	manager := defaultManager
	defaultManager = nil
	defaultMu.Unlock()
	if manager == nil {
		return nil
	}
	return manager.Close()
}
