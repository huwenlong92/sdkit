package eventbus

import "time"

type PublishOptions struct {
	Name       string
	Headers    map[string]string
	TraceID    string
	Delay      time.Duration
	TTL        time.Duration
	Retry      int
	Persistent bool
}

type PublishOption func(*PublishOptions)

func WithName(name string) PublishOption {
	return func(o *PublishOptions) {
		o.Name = name
	}
}

func WithHeaders(headers map[string]string) PublishOption {
	return func(o *PublishOptions) {
		o.Headers = cloneHeaders(headers)
	}
}

func WithHeader(key, value string) PublishOption {
	return func(o *PublishOptions) {
		if o.Headers == nil {
			o.Headers = make(map[string]string)
		}
		o.Headers[key] = value
	}
}

func WithTraceID(traceID string) PublishOption {
	return func(o *PublishOptions) {
		o.TraceID = traceID
	}
}

func WithDelay(delay time.Duration) PublishOption {
	return func(o *PublishOptions) {
		o.Delay = delay
	}
}

func WithTTL(ttl time.Duration) PublishOption {
	return func(o *PublishOptions) {
		o.TTL = ttl
	}
}

func WithRetry(retry int) PublishOption {
	return func(o *PublishOptions) {
		o.Retry = retry
	}
}

func WithPersistent(persistent bool) PublishOption {
	return func(o *PublishOptions) {
		o.Persistent = persistent
	}
}

func ApplyPublishOptions(opts ...PublishOption) PublishOptions {
	var options PublishOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return options
}

func CheckPublishCapability(options PublishOptions, capability Capability) error {
	if options.Delay > 0 && !capability.Delay {
		return ErrUnsupported
	}
	if options.TTL > 0 && !capability.Persistent {
		return ErrUnsupported
	}
	if options.Retry > 0 && !capability.Retry {
		return ErrUnsupported
	}
	if options.Persistent && !capability.Persistent {
		return ErrUnsupported
	}
	return nil
}

type SubscribeOptions struct {
	Group       string
	Durable     bool
	Concurrency int
}

type SubscribeOption func(*SubscribeOptions)

func WithGroup(group string) SubscribeOption {
	return func(o *SubscribeOptions) {
		o.Group = group
	}
}

func WithDurable(durable bool) SubscribeOption {
	return func(o *SubscribeOptions) {
		o.Durable = durable
	}
}

func WithConcurrency(concurrency int) SubscribeOption {
	return func(o *SubscribeOptions) {
		o.Concurrency = concurrency
	}
}

func ApplySubscribeOptions(opts ...SubscribeOption) SubscribeOptions {
	options := SubscribeOptions{Concurrency: 1}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if options.Concurrency <= 0 {
		options.Concurrency = 1
	}
	return options
}

func CheckSubscribeCapability(options SubscribeOptions, capability Capability) error {
	if options.Group != "" && !capability.ConsumerGrp {
		return ErrUnsupported
	}
	if options.Durable && !capability.Persistent {
		return ErrUnsupported
	}
	return nil
}
