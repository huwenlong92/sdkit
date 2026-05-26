package payment

import "errors"

var (
	ErrNilContext             = errors.New("payment: nil context")
	ErrInvalidAmount          = errors.New("payment: invalid amount")
	ErrInvalidRequest         = errors.New("payment: invalid request")
	ErrInvalidAction          = errors.New("payment: invalid action")
	ErrInvalidCurrency        = errors.New("payment: invalid currency")
	ErrInvalidExchangeRate    = errors.New("payment: invalid exchange rate")
	ErrCurrencyMetaNotFound   = errors.New("payment: currency metadata not found")
	ErrSettleAmountMismatch   = errors.New("payment: settle amount mismatch")
	ErrUnsupportedProvider    = errors.New("payment: unsupported provider")
	ErrUnsupportedChannel     = errors.New("payment: unsupported channel")
	ErrUnsupportedCapability  = errors.New("payment: unsupported capability")
	ErrAdapterAlreadyExists   = errors.New("payment: adapter already exists")
	ErrAdapterNotFound        = errors.New("payment: adapter not found")
	ErrPaymentActionRequired  = errors.New("payment: action required")
	ErrNotifyVerificationFail = errors.New("payment: notify verification failed")
	ErrPaymentReference       = errors.New("payment: payment reference required")
	ErrInvalidStateTransition = errors.New("payment: invalid state transition")
)
