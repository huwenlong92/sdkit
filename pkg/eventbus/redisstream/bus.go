package redisstream

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/huwenlong92/sdkit/core/eventbus"

	goredis "github.com/redis/go-redis/v9"
)

var ErrClosed = eventbus.ErrClosed

type Bus struct {
	rdb         *goredis.Client
	prefix      string
	group       string
	consumer    string
	maxLen      int64
	blockTime   time.Duration
	codec       eventbus.Codec
	middlewares []eventbus.Middleware

	mu      sync.Mutex
	closed  bool
	cancels []context.CancelFunc
}

type Option func(*Bus)

func WithMaxLen(maxLen int64) Option {
	return func(b *Bus) {
		b.maxLen = maxLen
	}
}

func WithBlockTime(blockTime time.Duration) Option {
	return func(b *Bus) {
		if blockTime > 0 {
			b.blockTime = blockTime
		}
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

func New(rdb *goredis.Client, prefix, group, consumer string, opts ...Option) *Bus {
	if group == "" {
		group = "default"
	}
	if consumer == "" {
		consumer = group
	}
	b := &Bus{
		rdb:         rdb,
		prefix:      strings.Trim(prefix, ":"),
		group:       sanitize(group),
		consumer:    sanitize(consumer),
		maxLen:      10000,
		blockTime:   5 * time.Second,
		codec:       eventbus.JSONCodec{},
		middlewares: []eventbus.Middleware{eventbus.Recover(nil), eventbus.Tracing()},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(b)
		}
	}
	return b
}

func (b *Bus) Publish(ctx context.Context, event *eventbus.Event) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := b.ensureOpen(); err != nil {
		return err
	}
	if b.rdb == nil {
		return errors.New("eventbus redis stream client is nil")
	}
	if event == nil || event.Topic == "" {
		return eventbus.ErrEmptyTopic
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	args := &goredis.XAddArgs{
		Stream: b.topic(event.Topic),
		Values: map[string]any{"event": string(data)},
	}
	if b.maxLen > 0 {
		args.MaxLen = b.maxLen
		args.Approx = true
	}
	return b.rdb.XAdd(ctx, args).Err()
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
		return nil, errors.New("eventbus redis stream client is nil")
	}

	stream := b.topic(topic)
	if err := b.ensureGroup(ctx, stream); err != nil {
		return nil, err
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

	wrapped := eventbus.Chain(handler, b.middlewares...)
	go b.consume(subCtx, stream, topic, wrapped)
	var once sync.Once
	return eventbus.SubscriptionFunc(func() error {
		once.Do(cancel)
		return nil
	}), nil
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

func (b *Bus) ensureGroup(ctx context.Context, stream string) error {
	err := b.rdb.XGroupCreateMkStream(ctx, stream, b.group, "$").Err()
	if err == nil || strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}
	return err
}

func (b *Bus) consume(ctx context.Context, stream, topic string, handler eventbus.Handler) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		streams, err := b.rdb.XReadGroup(ctx, &goredis.XReadGroupArgs{
			Group:    b.group,
			Consumer: b.consumer,
			Streams:  []string{stream, ">"},
			Count:    16,
			Block:    b.blockTime,
		}).Result()
		if errors.Is(err, goredis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(200 * time.Millisecond)
			continue
		}
		for _, s := range streams {
			for _, msg := range s.Messages {
				event := streamEvent(topic, msg.Values)
				if err := eventbus.SafeHandle(ctx, event, handler, nil); err != nil {
					continue
				}
				_ = b.rdb.XAck(ctx, stream, b.group, msg.ID).Err()
			}
		}
	}
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
		Persistent:  true,
		ConsumerGrp: true,
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

func sanitize(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, ":", "_")
	v = strings.ReplaceAll(v, " ", "_")
	if v == "" {
		return "default"
	}
	return v
}

func streamEvent(topic string, values map[string]any) *eventbus.Event {
	if raw, ok := values["event"].(string); ok && raw != "" {
		var event eventbus.Event
		if err := json.Unmarshal([]byte(raw), &event); err == nil && event.Payload != nil {
			if event.Topic == "" {
				event.Topic = topic
			}
			return &event
		}
	}
	if payload, ok := values["payload"].(string); ok {
		return &eventbus.Event{Topic: topic, Payload: []byte(payload)}
	}
	return &eventbus.Event{Topic: topic}
}
