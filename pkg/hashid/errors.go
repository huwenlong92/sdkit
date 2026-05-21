package hashid

import "errors"

var (
	ErrInvalidConfig = errors.New("hashid: invalid config")
	ErrTypeMismatch  = errors.New("hashid: mismatched type")
	ErrInvalidValue  = errors.New("hashid: invalid value")
)
