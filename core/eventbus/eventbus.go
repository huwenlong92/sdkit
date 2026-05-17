// Package eventbus defines a small publish/subscribe abstraction.
package eventbus

import "context"

type Handler func(ctx context.Context, event *Event) error

type Subscription interface {
	Close() error
}

type SubscriptionFunc func() error

func (fn SubscriptionFunc) Close() error {
	if fn == nil {
		return nil
	}
	return fn()
}

type Bus interface {
	Publish(ctx context.Context, event *Event) error
	Subscribe(ctx context.Context, topic string, handler Handler) (Subscription, error)
	Close() error
	Capability() Capability
}

type Capability struct {
	Fanout      bool
	Wildcard    bool
	Persistent  bool
	Delay       bool
	Retry       bool
	ConsumerGrp bool
}

type Service interface {
	Bus
}

type EventBus = Bus
