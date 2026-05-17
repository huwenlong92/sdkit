package realtime

import "errors"

var (
	ErrNilClient                = errors.New("realtime: nil client")
	ErrEmptyClientID            = errors.New("realtime: empty client id")
	ErrEmptyEvent               = errors.New("realtime: empty event")
	ErrNilPublisher             = errors.New("realtime: nil publisher")
	ErrNilGateway               = errors.New("realtime: nil gateway")
	ErrNilSubscriber            = errors.New("realtime: nil subscriber")
	ErrNilRouter                = errors.New("realtime: nil router")
	ErrNilActionCodec           = errors.New("realtime: nil action codec")
	ErrNilActionHandler         = errors.New("realtime: nil action handler")
	ErrEmptyAction              = errors.New("realtime: empty action")
	ErrEmptyIdentity            = errors.New("realtime: empty identity")
	ErrClientNotFound           = errors.New("realtime: client not found")
	ErrClientChannelUnavailable = errors.New("realtime: client channel unavailable")
	ErrClientBufferFull         = errors.New("realtime: client buffer full")
	ErrActionAlreadyRegistered  = errors.New("realtime: action already registered")
	ErrActionNotFound           = errors.New("realtime: action not found")
	ErrInvalidAction            = errors.New("realtime: invalid action")
	ErrDefaultNotReady          = errors.New("realtime: default not initialized")
	ErrUnsupportedTopic         = errors.New("realtime: unsupported topic")
	ErrClosed                   = errors.New("realtime: closed")
)
