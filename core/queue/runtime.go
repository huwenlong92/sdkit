package queue

import (
	"time"
)

type RetryDelayFunc func(retryCount int, err error, msg *Message) time.Duration

type IsFailureFunc func(error) bool

type RuntimeOption func(*RuntimeOptions)

type RuntimeOptions struct {
	IsFailure IsFailureFunc
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
