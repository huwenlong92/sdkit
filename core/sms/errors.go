package sms

import (
	"errors"
	"fmt"

	pkgsms "github.com/huwenlong92/sdkit/pkg/sms"
)

var (
	ErrNotConfigured       = errors.New("sms: not configured")
	ErrDefaultRequired     = errors.New("sms: default provider is required")
	ErrProviderNotFound    = errors.New("sms: provider not found")
	ErrDriverRequired      = pkgsms.ErrDriverRequired
	ErrUnknownDriver       = pkgsms.ErrUnknownDriver
	ErrNoProviderAvailable = errors.New("sms: no provider available")
	ErrProviderFailed      = errors.New("sms: provider send failed")
	ErrRateLimited         = errors.New("sms: rate limited")
)

type NoProviderAvailableError struct {
	Attempts []AttemptResult
}

func (e *NoProviderAvailableError) Error() string {
	return ErrNoProviderAvailable.Error()
}

func (e *NoProviderAvailableError) Is(target error) bool {
	return target == ErrNoProviderAvailable
}

func (e *NoProviderAvailableError) LastError() error {
	if e == nil {
		return nil
	}
	for i := len(e.Attempts) - 1; i >= 0; i-- {
		if e.Attempts[i].Error != nil {
			return e.Attempts[i].Error
		}
	}
	return nil
}

func providerNotFound(name string) error {
	return fmt.Errorf("%w: %s", ErrProviderNotFound, name)
}
