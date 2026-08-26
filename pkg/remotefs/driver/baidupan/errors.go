package baidupan

import "errors"

var (
	ErrBinaryUnavailable    = errors.New("baidupan: binary unavailable")
	ErrVersionUnsupported   = errors.New("baidupan: version unsupported")
	ErrShareInvalid         = errors.New("baidupan: share invalid")
	ErrShareExpired         = errors.New("baidupan: share expired")
	ErrSharePasswordInvalid = errors.New("baidupan: share password invalid")
)
