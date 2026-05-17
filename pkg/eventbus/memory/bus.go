package memory

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/huwenlong92/sdkit/core/eventbus"
)

var ErrClosed = eventbus.ErrClosed

type Option func(*Bus)

func WithCodec(codec eventbus.Codec) Option {
	return func(b *Bus) {
		if codec != nil {
			b.codec = codec
		}
	}
}

func WithMiddleware(middlewares ...eventbus.Middleware) Option {
	return func(b *Bus) {
		b.middlewares = append(b.middlewares, middlewares...)
	}
}

type subscriber struct {
	id      string
	ctx     context.Context
	cancel  context.CancelFunc
	handler eventbus.Handler
}

type Bus struct {
	mu          sync.RWMutex
	closed      bool
	nextID      uint64
	codec       eventbus.Codec
	middlewares []eventbus.Middleware
	subscribers map[string]map[string]subscriber
}

func New(opts ...Option) *Bus {
	bus := &Bus{
		codec:       eventbus.JSONCodec{},
		middlewares: []eventbus.Middleware{eventbus.Recover(nil), eventbus.Tracing()},
		subscribers: make(map[string]map[string]subscriber),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(bus)
		}
	}
	return bus
}

func (b *Bus) Publish(ctx context.Context, event *eventbus.Event) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if event == nil || event.Topic == "" {
		return eventbus.ErrEmptyTopic
	}

	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return ErrClosed
	}
	subscribers := make([]subscriber, 0, len(b.subscribers[event.Topic]))
	for _, sub := range b.subscribers[event.Topic] {
		subscribers = append(subscribers, sub)
	}
	b.mu.RUnlock()

	var firstErr error
	for _, sub := range subscribers {
		if sub.ctx.Err() != nil {
			continue
		}
		handler := eventbus.Chain(sub.handler, b.middlewares...)
		if err := eventbus.SafeHandle(ctx, event, handler, nil); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (b *Bus) Subscribe(ctx context.Context, topic string, handler eventbus.Handler) (eventbus.Subscription, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if topic == "" {
		return nil, eventbus.ErrEmptyTopic
	}
	if handler == nil {
		return nil, eventbus.ErrNilHandler
	}

	id := b.nextSubscriberID()
	subCtx, cancel := context.WithCancel(ctx)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		cancel()
		return nil, ErrClosed
	}
	if b.subscribers[topic] == nil {
		b.subscribers[topic] = make(map[string]subscriber)
	}
	b.subscribers[topic][id] = subscriber{
		id:      id,
		ctx:     subCtx,
		cancel:  cancel,
		handler: handler,
	}
	b.mu.Unlock()

	var once sync.Once
	unsubscribe := eventbus.SubscriptionFunc(func() error {
		once.Do(func() {
			cancel()
			b.mu.Lock()
			if subscribers := b.subscribers[topic]; subscribers != nil {
				delete(subscribers, id)
				if len(subscribers) == 0 {
					delete(b.subscribers, topic)
				}
			}
			b.mu.Unlock()
		})
		return nil
	})
	go func() {
		<-subCtx.Done()
		_ = unsubscribe.Close()
	}()
	return unsubscribe, nil
}

func (b *Bus) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	subscribers := b.subscribers
	b.subscribers = nil
	b.mu.Unlock()

	for _, topicSubscribers := range subscribers {
		for _, sub := range topicSubscribers {
			sub.cancel()
		}
	}
	return nil
}

func (b *Bus) Capability() eventbus.Capability {
	return eventbus.Capability{
		Fanout: true,
	}
}

func (b *Bus) nextSubscriberID() string {
	id := atomic.AddUint64(&b.nextID, 1)
	return "memory-" + strconv.FormatUint(id, 10)
}
