package tracing

import (
	"context"
	"sync"
	"sync/atomic"
)

var (
	shutdownMu     sync.Mutex
	globalShutdown = noopShutdown
	enabled        atomic.Bool
)

func Enabled() bool {
	return enabled.Load()
}

func Shutdown(ctx context.Context) error {
	shutdownMu.Lock()
	shutdown := globalShutdown
	globalShutdown = noopShutdown
	shutdownMu.Unlock()
	enabled.Store(false)
	return shutdown(ctx)
}

func setShutdown(shutdown func(context.Context) error) {
	if shutdown == nil {
		shutdown = noopShutdown
	}
	shutdownMu.Lock()
	globalShutdown = shutdown
	shutdownMu.Unlock()
}

func noopShutdown(context.Context) error {
	return nil
}
