package queue

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotInitialized        = errors.New("queue: not initialized")
	ErrCapabilityUnsupported = errors.New("queue capability unsupported")
	ErrTaskDuplicated        = errors.New("queue task duplicated")
	ErrTaskNotFound          = errors.New("queue task not found")
	ErrQueueNotFound         = errors.New("queue not found")
	ErrQueuePaused           = errors.New("queue paused")
	ErrRateLimited           = errors.New("queue rate limited")
	ErrTaskCanceled          = errors.New("queue task canceled")
	ErrInvalidPayload        = errors.New("queue invalid payload")
	ErrHandlerNotFound       = errors.New("queue handler not found")
	ErrLockNotAcquired       = errors.New("queue lock not acquired")
	ErrIdempotentDone        = errors.New("queue idempotent done")
)

type RateLimitError struct {
	RetryIn time.Duration
	Err     error
}

type ErrorKind string

const (
	ErrorRetryable  ErrorKind = "retryable"
	ErrorFatal      ErrorKind = "fatal"
	ErrorTimeout    ErrorKind = "timeout"
	ErrorDeadLetter ErrorKind = "deadletter"
	ErrorIgnored    ErrorKind = "ignored"
)

type RuntimeError struct {
	Kind      ErrorKind
	Code      string
	Retryable bool
	Fatal     bool
	Ignore    bool
	RetryIn   time.Duration
	Err       error
}

func (e *RuntimeError) Error() string {
	if e == nil {
		return "queue runtime error"
	}
	if e.Code != "" && e.Err != nil {
		return fmt.Sprintf("queue runtime error %s: %v", e.Code, e.Err)
	}
	if e.Code != "" {
		return "queue runtime error " + e.Code
	}
	if e.Err != nil {
		return "queue runtime error: " + e.Err.Error()
	}
	return "queue runtime error"
}

func (e *RuntimeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *RateLimitError) Error() string {
	if e == nil {
		return "queue: rate limited"
	}
	if e.Err != nil {
		return fmt.Sprintf("queue: rate limited, retry in %s: %v", e.RetryIn, e.Err)
	}
	return fmt.Sprintf("queue: rate limited, retry in %s", e.RetryIn)
}

func (e *RateLimitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func RateLimited(retryIn time.Duration, err error) error {
	return &RateLimitError{RetryIn: retryIn, Err: errors.Join(ErrRateLimited, err)}
}

func IsRateLimitError(err error) bool {
	if errors.Is(err, ErrRateLimited) {
		return true
	}
	var target *RateLimitError
	return errors.As(err, &target)
}

func IsLockNotAcquired(err error) bool {
	return errors.Is(err, ErrLockNotAcquired)
}

func NewRuntimeError(code string, err error) error {
	return &RuntimeError{Code: code, Err: err}
}

func NewRetryableError(err error) error {
	return RetryableAfter(0, err)
}

func RetryableAfter(retryIn time.Duration, err error) error {
	return &RuntimeError{
		Kind:      ErrorRetryable,
		Code:      "retryable",
		Retryable: true,
		RetryIn:   retryIn,
		Err:       err,
	}
}

func NewFatalError(err error) error {
	return &RuntimeError{
		Kind:  ErrorFatal,
		Code:  "fatal",
		Fatal: true,
		Err:   err,
	}
}

func NewIgnoredError(err error) error {
	return &RuntimeError{
		Kind:   ErrorIgnored,
		Code:   "ignored",
		Ignore: true,
		Err:    err,
	}
}

func NewTimeoutError(err error) error {
	return &RuntimeError{
		Kind:      ErrorTimeout,
		Code:      "timeout",
		Retryable: true,
		RetryIn:   0,
		Err:       err,
	}
}

func NewDeadLetterError(err error) error {
	return &RuntimeError{
		Kind:  ErrorDeadLetter,
		Code:  "deadletter",
		Fatal: true,
		Err:   err,
	}
}

func RuntimeErrorFrom(err error) (*RuntimeError, bool) {
	if err == nil {
		return nil, false
	}
	var runtimeErr *RuntimeError
	if errors.As(err, &runtimeErr) && runtimeErr != nil {
		return runtimeErr, true
	}
	return nil, false
}

func IsRetryableError(err error) bool {
	runtimeErr, ok := RuntimeErrorFrom(err)
	return ok && (runtimeErr.Retryable || runtimeErr.Kind == ErrorRetryable)
}

func IsFatalError(err error) bool {
	runtimeErr, ok := RuntimeErrorFrom(err)
	return ok && (runtimeErr.Fatal || runtimeErr.Kind == ErrorFatal)
}

func IsIgnoredError(err error) bool {
	runtimeErr, ok := RuntimeErrorFrom(err)
	return ok && (runtimeErr.Ignore || runtimeErr.Kind == ErrorIgnored)
}

func IsTimeoutError(err error) bool {
	runtimeErr, ok := RuntimeErrorFrom(err)
	return ok && runtimeErr.Kind == ErrorTimeout
}

func IsDeadLetterError(err error) bool {
	runtimeErr, ok := RuntimeErrorFrom(err)
	return ok && runtimeErr.Kind == ErrorDeadLetter
}
