package email

import (
	"errors"
	"fmt"

	pkgemail "github.com/huwenlong92/sdkit/pkg/email"
)

var (
	ErrNotConfigured       = errors.New("email: not configured")
	ErrDefaultRequired     = errors.New("email: default provider is required")
	ErrProviderNotFound    = errors.New("email: provider not found")
	ErrMessageRequired     = pkgemail.ErrMessageRequired
	ErrDriverRequired      = pkgemail.ErrDriverRequired
	ErrUnknownDriver       = pkgemail.ErrUnknownDriver
	ErrNoProviderAvailable = errors.New("email: no provider available")
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
