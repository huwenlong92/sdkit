package sms

import (
	"context"
	"sync"
)

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

func Send(ctx context.Context, to []string, msg Message) (*SendResult, error) {
	manager, err := ManagerDefault()
	if err != nil {
		return nil, err
	}
	return manager.Send(ctx, to, msg)
}

func SendVia(ctx context.Context, to []string, msg Message, providers ...string) (*SendResult, error) {
	manager, err := ManagerDefault()
	if err != nil {
		return nil, err
	}
	return manager.SendVia(ctx, to, msg, providers...)
}

func Use(name string) (Provider, error) {
	manager, err := ManagerDefault()
	if err != nil {
		return nil, err
	}
	return manager.Use(name)
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
