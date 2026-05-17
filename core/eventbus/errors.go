package eventbus

import "errors"

var (
	ErrDefaultNotInitialized = errors.New("eventbus: default not initialized")
	ErrClosed                = errors.New("eventbus: closed")
	ErrUnsupported           = errors.New("eventbus: unsupported capability")
	ErrBusNotFound           = errors.New("eventbus: bus not found")
	ErrNilHandler            = errors.New("eventbus: nil handler")
	ErrEmptyTopic            = errors.New("eventbus: empty topic")
)
