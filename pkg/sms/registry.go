package sms

import (
	"errors"
	"strings"
	"sync"
)

var (
	ErrDriverRequired = errors.New("sms: driver is required")
	ErrUnknownDriver  = errors.New("sms: unknown driver")
)

type DriverFactory func(name string, cfg ProviderConfig) (Provider, error)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]DriverFactory)
)

func RegisterDriver(driver string, factory DriverFactory) {
	driver = strings.TrimSpace(driver)
	if driver == "" || factory == nil {
		return
	}
	registryMu.Lock()
	registry[driver] = factory
	registryMu.Unlock()
}

func NewProvider(name string, cfg ProviderConfig) (Provider, error) {
	driver := strings.TrimSpace(cfg.Driver)
	if driver == "" {
		return nil, ErrDriverRequired
	}
	registryMu.RLock()
	factory := registry[driver]
	registryMu.RUnlock()
	if factory == nil {
		return nil, ErrUnknownDriver
	}
	return factory(name, cfg)
}
