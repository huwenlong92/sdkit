package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/huwenlong92/sdkit/core/eventbus"

	natsgo "github.com/nats-io/nats.go"
)

var ErrClosed = eventbus.ErrClosed

type Option func(*Bus)

func WithConnection(conn *natsgo.Conn) Option {
	return func(b *Bus) {
		b.conn = conn
		b.ownsConn = false
	}
}

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
	conn          *natsgo.Conn
	ownsConn      bool
	subjectPrefix string
	codec         eventbus.Codec
	middlewares   []eventbus.Middleware

	mu     sync.Mutex
	closed bool
	subs   []*natsgo.Subscription
}

func New(addr string, subjectPrefix string, opts ...Option) (*Bus, error) {
	bus := &Bus{
		ownsConn:      true,
		subjectPrefix: strings.Trim(subjectPrefix, "."),
		codec:         eventbus.JSONCodec{},
		middlewares:   []eventbus.Middleware{eventbus.Recover(nil), eventbus.Tracing()},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(bus)
		}
	}
	if bus.conn != nil {
		return bus, nil
	}
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, fmt.Errorf("eventbus nats addr is required")
	}
	conn, err := natsgo.Connect(addr)
	if err != nil {
		return nil, err
	}
	bus.conn = conn
	bus.ownsConn = true
	return bus, nil
}

func (b *Bus) Publish(ctx context.Context, event *eventbus.Event) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := b.ensureOpen(); err != nil {
		return err
	}
	if b.conn == nil {
		return errors.New("eventbus nats connection is nil")
	}
	if event == nil || event.Topic == "" {
		return eventbus.ErrEmptyTopic
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if err := b.conn.Publish(b.subject(event.Topic), data); err != nil {
		return err
	}
	return b.conn.FlushWithContext(ctx)
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
	if b.conn == nil {
		return nil, errors.New("eventbus nats connection is nil")
	}

	subCtx, cancel := context.WithCancel(ctx)
	wrapped := eventbus.Chain(handler, b.middlewares...)
	subscription, err := b.conn.Subscribe(b.subject(topic), func(msg *natsgo.Msg) {
		event := decodeEvent(topic, msg.Data)
		_ = eventbus.SafeHandle(subCtx, event, wrapped, nil)
	})
	if err != nil {
		cancel()
		return nil, err
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		cancel()
		_ = subscription.Unsubscribe()
		return nil, ErrClosed
	}
	b.subs = append(b.subs, subscription)
	b.mu.Unlock()

	var once sync.Once
	unsubscribe := eventbus.SubscriptionFunc(func() error {
		once.Do(func() {
			cancel()
			_ = subscription.Unsubscribe()
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
	subs := append([]*natsgo.Subscription(nil), b.subs...)
	b.subs = nil
	conn := b.conn
	ownsConn := b.ownsConn
	b.mu.Unlock()

	for _, subscription := range subs {
		if subscription != nil {
			_ = subscription.Unsubscribe()
		}
	}
	if conn != nil && ownsConn {
		conn.Close()
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

func (b *Bus) subject(topic string) string {
	topic = strings.Trim(topic, ".")
	if b.subjectPrefix == "" {
		return topic
	}
	if topic == "" {
		return b.subjectPrefix
	}
	return b.subjectPrefix + "." + topic
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
