package queue

import (
	"context"
	"time"
)

type Failure struct {
	TaskID      string
	Queue       string
	Type        string
	Payload     []byte
	Err         error
	RetryCount  int
	MaxRetry    int
	RateLimited bool
	Headers     map[string]string
}

type FailureHandler func(context.Context, *Failure)

type RetryDelayFunc func(retryCount int, err error, msg *Message) time.Duration

type IsFailureFunc func(error) bool

type RuntimeOption func(*RuntimeOptions)

type RuntimeOptions struct {
	FailureHandler FailureHandler
	IsFailure      IsFailureFunc
}

func WithFailureHandler(handler FailureHandler) RuntimeOption {
	return func(o *RuntimeOptions) {
		o.FailureHandler = handler
	}
}

func WithIsFailure(fn IsFailureFunc) RuntimeOption {
	return func(o *RuntimeOptions) {
		o.IsFailure = fn
	}
}

func DefaultRuntimeOptions() RuntimeOptions {
	return RuntimeOptions{
		IsFailure: func(err error) bool {
			return !IsRateLimitError(err) && !IsIgnoredError(err)
		},
	}
}

func ApplyRuntimeOptions(opts []RuntimeOption) RuntimeOptions {
	out := DefaultRuntimeOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}
	return out
}
