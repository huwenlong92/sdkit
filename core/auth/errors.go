package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountDisabled    = errors.New("account disabled")
	ErrUnauthorized       = errors.New("unauthorized")
)
