package eventbus

import (
	"context"
	"strings"
	"sync"
)

var (
	defaultMu     sync.RWMutex
	defaultBus    Service
	defaultDriver string
	manager       = NewManager()
)

func SetDefault(bus Service) {
	SetDefaultWithDriver(bus, "")
}

func SetDefaultWithDriver(bus Service, driver string) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultBus = bus
	defaultDriver = normalizeDriver(driver)
}

func Default() Service {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultBus
}

func DefaultWithDriver() (Service, string) {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultBus, defaultDriver
}

func DefaultDriver() string {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultDriver
}

func Publish(ctx context.Context, event *Event) error {
	bus := Default()
	if bus == nil {
		return ErrDefaultNotInitialized
	}
	return bus.Publish(ctx, event)
}

func Subscribe(ctx context.Context, topic string, handler Handler) (Subscription, error) {
	bus := Default()
	if bus == nil {
		return nil, ErrDefaultNotInitialized
	}
	return bus.Subscribe(ctx, topic, handler)
}

func CloseDefault() error {
	defaultMu.Lock()
	bus := defaultBus
	defaultBus = nil
	defaultDriver = ""
	defaultMu.Unlock()
	if bus == nil {
		return nil
	}
	return bus.Close()
}

func Register(name string, bus Service) error {
	return manager.Register(name, bus)
}

func Get(name string) (Service, error) {
	return manager.Get(name)
}

func MustGet(name string) Service {
	return manager.MustGet(name)
}

func normalizeDriver(driver string) string {
	return strings.ToLower(strings.TrimSpace(driver))
}
