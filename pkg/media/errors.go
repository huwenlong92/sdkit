package media

import (
	"errors"
	"strings"
)

var (
	ErrInvalidInput       = errors.New("media: invalid input")
	ErrInputUnavailable   = errors.New("media: input unavailable")
	ErrBinaryUnavailable  = errors.New("media: binary unavailable")
	ErrVersionUnsupported = errors.New("media: version unsupported")
	ErrProbeFailed        = errors.New("media: probe failed")
	ErrOutputInvalid      = errors.New("media: invalid probe output")
	ErrOutputLimit        = errors.New("media: output limit exceeded")
)

type Error struct {
	Operation string
	Summary   string
	Err       error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	prefix := "media"
	if operation := strings.TrimSpace(e.Operation); operation != "" {
		prefix += ": " + operation
	}
	if summary := safeSummary(e.Summary); summary != "" {
		return prefix + ": " + summary
	}
	if e.Err != nil {
		return prefix + ": " + e.Err.Error()
	}
	return prefix
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func safeSummary(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	const maxBytes = 512
	if len(value) > maxBytes {
		return value[:maxBytes] + "..."
	}
	return value
}
