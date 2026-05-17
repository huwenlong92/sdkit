package redis

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/huwenlong92/sdkit/core/eventbus"

	goredis "github.com/redis/go-redis/v9"
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

type Bus struct {
	rdb         *goredis.Client
	prefix      string
	codec       eventbus.Codec
	middlewares []eventbus.Middleware

	mu      sync.Mutex
	closed  bool
	cancels []context.CancelFunc
}

func New(rdb *goredis.Client, prefix string, opts ...Option) *Bus {
	bus := &Bus{
		rdb:         rdb,
		prefix:      strings.Trim(prefix, ":"),
		codec:       eventbus.JSONCodec{},
		middlewares: []eventbus.Middleware{eventbus.Recover(nil), eventbus.Tracing()},
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
	if err := b.ensureOpen(); err != nil {
		return err
	}
	if b.rdb == nil {
		return errors.New("eventbus redis client is nil")
	}
	if event == nil || event.Topic == "" {
		return eventbus.ErrEmptyTopic
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return b.rdb.Publish(ctx, b.topic(event.Topic), data).Err()
}

func (b *Bus) Subscribe(ctx context.Context, topic string, handler eventbus.Handler) (eventbus.Subscription, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := b.ensureOpen(); err != nil {
		return nil, err
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
	if b.rdb == nil {
		return nil, errors.New("eventbus redis client is nil")
	}

	subCtx, cancel := context.WithCancel(ctx)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		cancel()
		return nil, ErrClosed
	}
	b.cancels = append(b.cancels, cancel)
	b.mu.Unlock()

	pubsub := b.rdb.Subscribe(subCtx, b.topic(topic))
	if err := pubsub.Ping(subCtx); err != nil {
		cancel()
		_ = pubsub.Close()
		return nil, err
	}

	wrapped := eventbus.Chain(handler, b.middlewares...)
	var once sync.Once
	unsubscribe := eventbus.SubscriptionFunc(func() error {
		once.Do(func() {
			cancel()
			_ = pubsub.Close()
		})
		return nil
	})
	go func() {
		defer pubsub.Close()
		ch := pubsub.Channel()
		for {
			select {
			case <-subCtx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				event := decodeEvent(topic, []byte(msg.Payload))
				_ = eventbus.SafeHandle(subCtx, event, wrapped, nil)
			}
		}
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
	cancels := append([]context.CancelFunc(nil), b.cancels...)
	b.cancels = nil
	b.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	return nil
}

func (b *Bus) ensureOpen() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrClosed
	}
	return nil
}

func (b *Bus) Capability() eventbus.Capability {
	return eventbus.Capability{
		Fanout: true,
	}
}

func (b *Bus) topic(topic string) string {
	topic = strings.Trim(topic, ":")
	if b.prefix == "" {
		return topic
	}
	if topic == "" {
		return b.prefix
	}
	return b.prefix + ":" + topic
}

func decodeEvent(topic string, data []byte) *eventbus.Event {
	var event eventbus.Event
	if err := json.Unmarshal(data, &event); err == nil && event.Payload != nil {
		if event.Topic == "" {
			event.Topic = topic
		}
		return &event
	}
	return &eventbus.Event{Topic: topic, Payload: append([]byte(nil), data...)}
}
