package storage

import (
	"strings"
	"sync"

	"github.com/huwenlong92/sdkit/pkg/storage/core"
	"github.com/huwenlong92/sdkit/pkg/storage/driver/local"
)

type DriverFactory func(cfg core.Config) (core.Handler, error)

var (
	driverMu sync.RWMutex
	drivers  = map[string]DriverFactory{}
)

func init() {
	RegisterDriver("local", func(cfg core.Config) (core.Handler, error) {
		return local.NewFromConfig(cfg), nil
	})
}

func RegisterDriver(driver string, factory DriverFactory) {
	driver = strings.TrimSpace(driver)
	if driver == "" || factory == nil {
		return
	}
	driverMu.Lock()
	drivers[driver] = factory
	driverMu.Unlock()
}

func resolveDriver(driver string) DriverFactory {
	driver = strings.TrimSpace(driver)
	if driver == "" {
		return nil
	}
	driverMu.RLock()
	factory := drivers[driver]
	driverMu.RUnlock()
	return factory
}
