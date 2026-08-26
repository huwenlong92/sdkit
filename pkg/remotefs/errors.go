package remotefs

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrNotFound         = errors.New("remotefs: not found")
	ErrUnauthenticated  = errors.New("remotefs: unauthenticated")
	ErrPermissionDenied = errors.New("remotefs: permission denied")
	ErrRateLimited      = errors.New("remotefs: rate limited")
	ErrQuotaExceeded    = errors.New("remotefs: quota exceeded")
	ErrConflict         = errors.New("remotefs: conflict")
	ErrUnsupported      = errors.New("remotefs: unsupported")
	ErrTemporary        = errors.New("remotefs: temporary failure")
	ErrNilContext       = errors.New("remotefs: nil context")
	ErrInvalidArgument  = errors.New("remotefs: invalid argument")
	ErrInvalidOption    = errors.New("remotefs: invalid option")
	ErrNotDirectory     = errors.New("remotefs: not a directory")
	ErrNotRegularFile   = errors.New("remotefs: not a regular file")
	ErrProtocol         = errors.New("remotefs: provider protocol error")
	ErrIntegrity        = errors.New("remotefs: integrity check failed")
	ErrSourceChanged    = errors.New("remotefs: source changed")
	ErrIdentityMismatch = errors.New("remotefs: provider identity mismatch")

	ErrInvalidReference  = errors.New("remotefs: invalid reference")
	ErrWalkLimitExceeded = errors.New("remotefs: walk entry limit exceeded")
	ErrClosed            = errors.New("remotefs: closed")
)

type Error struct {
	Operation string
	Driver    string
	Summary   string
	Err       error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	prefix := strings.TrimSpace(e.Driver)
	if prefix == "" {
		prefix = "remotefs"
	}
	if operation := strings.TrimSpace(e.Operation); operation != "" {
		prefix += ": " + operation
	}
	if summary := SafeSummary(e.Summary); summary != "" {
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

func WrapError(operation string, driver string, err error, summary string) error {
	if err == nil {
		return nil
	}
	return &Error{
		Operation: operation,
		Driver:    driver,
		Summary:   SafeSummary(summary),
		Err:       err,
	}
}

func SafeSummary(value string) string {
	value = strings.Join(strings.Fields(stripTerminalControls(value)), " ")
	const maxSummaryBytes = 512
	if len(value) > maxSummaryBytes {
		limit := maxSummaryBytes
		for limit > 0 && !utf8.RuneStart(value[limit]) {
			limit--
		}
		value = value[:limit] + "..."
	}
	return value
}

func stripTerminalControls(value string) string {
	var sanitized strings.Builder
	for index := 0; index < len(value); {
		r, size := utf8.DecodeRuneInString(value[index:])
		index += size
		if r == '\x1b' {
			if index < len(value) && value[index] == '[' {
				index++
				for index < len(value) {
					terminal := value[index]
					index++
					if terminal >= 0x40 && terminal <= 0x7e {
						break
					}
				}
			}
			continue
		}
		if unicode.IsControl(r) {
			sanitized.WriteByte(' ')
			continue
		}
		sanitized.WriteRune(r)
	}
	return sanitized.String()
}
