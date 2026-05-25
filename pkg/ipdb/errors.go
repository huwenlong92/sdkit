package ipdb

import "errors"

var (
	ErrInvalidConfig     = errors.New("ipdb: invalid config")
	ErrInvalidIP         = errors.New("ipdb: invalid ip")
	ErrUnsupportedDriver = errors.New("ipdb: unsupported driver")
	ErrClosed            = errors.New("ipdb: locator closed")
)
