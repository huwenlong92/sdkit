package realtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	coreeventbus "github.com/huwenlong92/sdkit/core/eventbus"
	corerealtime "github.com/huwenlong92/sdkit/core/realtime"
	eventbuspublisher "github.com/huwenlong92/sdkit/pkg/realtime/publisher/eventbus"
)

const Name = "realtime"

var (
	ErrNotConfigured         = errors.New("realtime capability 未初始化")
	ErrEventBusNotConfigured = errors.New("realtime capability requires eventbus default")
)

type Config struct {
	Topic string `mapstructure:"topic" yaml:"topic"`
}

type Service interface {
	corerealtime.Publisher
	Close() error
	Topic() string
}

type Capability struct {
	mu        sync.RWMutex
	publisher corerealtime.Publisher
	topic     string
}

type Option func(*options)

type options struct {
	publisher  corerealtime.Publisher
	eventbus   coreeventbus.Bus
	setDefault bool
}

var (
	defaultMu      sync.RWMutex
	defaultService Service
)

func WithPublisher(publisher corerealtime.Publisher) Option {
	return func(o *options) {
		o.publisher = publisher
	}
}

func WithEventBus(bus coreeventbus.Bus) Option {
	return func(o *options) {
		o.eventbus = bus
	}
}

func WithoutDefault() Option {
	return func(o *options) {
		o.setDefault = false
	}
}

func New(cfg Config, opts ...Option) (*Capability, error) {
	cfg = NormalizeConfig(cfg)
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	option := options{setDefault: true}
	for _, opt := range opts {
		if opt != nil {
			opt(&option)
		}
	}
	publisher, err := buildPublisher(cfg, option)
	if err != nil {
		return nil, err
	}
	capability := &Capability{publisher: publisher, topic: cfg.Topic}
	if option.setDefault {
		SetDefault(capability)
		corerealtime.SetDefaultPublisher(capability)
	}
	return capability, nil
}

func NormalizeConfig(cfg Config) Config {
	cfg.Topic = strings.TrimSpace(cfg.Topic)
	if cfg.Topic == "" {
		cfg.Topic = corerealtime.DefaultTopic
	}
	return cfg
}

func ValidateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.Topic) == "" {
		return fmt.Errorf("realtime topic: %w", coreeventbus.ErrEmptyTopic)
	}
	return nil
}

func buildPublisher(cfg Config, option options) (corerealtime.Publisher, error) {
	if option.publisher != nil {
		return option.publisher, nil
	}
	bus := option.eventbus
	if bus == nil {
		bus = coreeventbus.Default()
	}
	if bus == nil {
		return nil, fmt.Errorf("%w: %w", ErrEventBusNotConfigured, coreeventbus.ErrDefaultNotInitialized)
	}
	return eventbuspublisher.New(bus, cfg.Topic), nil
}

func SetDefault(service Service) {
	defaultMu.Lock()
	defaultService = service
	defaultMu.Unlock()
}

func Default() Service {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultService
}

func CloseDefault() error {
	defaultMu.Lock()
	service := defaultService
	defaultService = nil
	defaultMu.Unlock()
	if service == nil {
		return nil
	}
	return service.Close()
}

func PushUser(ctx context.Context, uid int64, event string, data any) error {
	service := Default()
	if service == nil {
		return ErrNotConfigured
	}
	return service.PushUser(ctx, fmt.Sprintf("%d", uid), corerealtime.NewEvent(event, data))
}

func PushRoom(ctx context.Context, roomID string, event string, data any) error {
	service := Default()
	if service == nil {
		return ErrNotConfigured
	}
	return service.PushRoom(ctx, roomID, corerealtime.NewEvent(event, data))
}

func Broadcast(ctx context.Context, event string, data any) error {
	service := Default()
	if service == nil {
		return ErrNotConfigured
	}
	return service.Broadcast(ctx, corerealtime.NewEvent(event, data))
}

func (c *Capability) PushUser(ctx context.Context, userID string, event *corerealtime.Event) error {
	publisher := c.publisherOrNil()
	if publisher == nil {
		return ErrNotConfigured
	}
	return publisher.PushUser(ctx, userID, event)
}

func (c *Capability) PushRoom(ctx context.Context, roomID string, event *corerealtime.Event) error {
	publisher := c.publisherOrNil()
	if publisher == nil {
		return ErrNotConfigured
	}
	return publisher.PushRoom(ctx, roomID, event)
}

func (c *Capability) Broadcast(ctx context.Context, event *corerealtime.Event) error {
	publisher := c.publisherOrNil()
	if publisher == nil {
		return ErrNotConfigured
	}
	return publisher.Broadcast(ctx, event)
}

func (c *Capability) Close() error {
	if c == nil {
		return nil
	}
	clearDefaultCapability(c)
	c.mu.Lock()
	c.publisher = nil
	c.topic = ""
	c.mu.Unlock()
	corerealtime.ClearDefaultPublisher(c)
	return nil
}

func (c *Capability) Topic() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.topic
}

func (c *Capability) publisherOrNil() corerealtime.Publisher {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.publisher
}

func clearDefaultCapability(capability *Capability) {
	defaultMu.Lock()
	if defaultService == capability {
		defaultService = nil
	}
	defaultMu.Unlock()
}

var _ Service = (*Capability)(nil)
