package eventbus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	coreeventbus "github.com/huwenlong92/sdkit/core/eventbus"
	eventbusmemory "github.com/huwenlong92/sdkit/pkg/eventbus/memory"
	eventbusnats "github.com/huwenlong92/sdkit/pkg/eventbus/nats"
	eventbusredis "github.com/huwenlong92/sdkit/pkg/eventbus/redis"
	eventbusstream "github.com/huwenlong92/sdkit/pkg/eventbus/redisstream"

	goredis "github.com/redis/go-redis/v9"
)

var ErrNotConfigured = errors.New("eventbus capability 未初始化")
var ErrRedisClientRequired = errors.New("eventbus capability requires redis client for redis driver")

type Service interface {
	coreeventbus.Service
	Bus() coreeventbus.Bus
	Driver() string
}

type Capability struct {
	mu                sync.RWMutex
	bus               coreeventbus.Bus
	driver            string
	ownsBus           bool
	registeredDefault bool
}

type Option func(*options)

type options struct {
	bus        coreeventbus.Bus
	driver     string
	redis      *goredis.Client
	ownsBus    bool
	setDefault bool
}

func WithBus(bus coreeventbus.Bus, driver string) Option {
	return func(o *options) {
		o.bus = bus
		o.driver = driver
		o.ownsBus = false
	}
}

func WithOwnedBus(bus coreeventbus.Bus, driver string) Option {
	return func(o *options) {
		o.bus = bus
		o.driver = driver
		o.ownsBus = true
	}
}

func WithRedisClient(redis *goredis.Client) Option {
	return func(o *options) {
		o.redis = redis
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

	if option.bus != nil {
		driver := normalizeDriver(option.driver)
		if driver == "" {
			driver = cfg.Driver
		}
		return attachBus(option.bus, driver, option.setDefault, option.ownsBus)
	}

	if bus, driver := coreeventbus.DefaultWithDriver(); bus != nil {
		driver = normalizeDriver(driver)
		if driver == "" {
			driver = DriverMemory
		}
		if err := ensureDriver(cfg.Driver, driver); err != nil {
			return nil, err
		}
		return &Capability{bus: bus, driver: driver}, nil
	}

	bus, err := buildBus(cfg, option)
	if err != nil {
		return nil, err
	}
	return attachBus(bus, cfg.Driver, option.setDefault, true)
}

func NormalizeConfig(cfg Config) Config {
	cfg.Driver = normalizeDriver(cfg.Driver)
	if cfg.Driver == "" {
		cfg.Driver = DriverMemory
	}
	cfg.Addr = strings.TrimSpace(cfg.Addr)
	cfg.TopicPrefix = strings.Trim(cfg.TopicPrefix, ":")
	cfg.SubjectPrefix = strings.Trim(cfg.SubjectPrefix, ".")
	cfg.NodeName = strings.TrimSpace(cfg.NodeName)
	return cfg
}

func ValidateConfig(cfg Config) error {
	driver := normalizeDriver(cfg.Driver)
	if driver == "" {
		return nil
	}
	switch driver {
	case DriverMemory, DriverRedis, DriverRedisStream, DriverNATS:
		return nil
	default:
		return fmt.Errorf("eventbus driver %q is invalid, want memory, redis, redis_stream, or nats", cfg.Driver)
	}
}

func (c *Capability) Publish(ctx context.Context, event *coreeventbus.Event) error {
	bus := c.Bus()
	if bus == nil {
		return ErrNotConfigured
	}
	return bus.Publish(ctx, event)
}

func (c *Capability) Subscribe(ctx context.Context, topic string, handler coreeventbus.Handler) (coreeventbus.Subscription, error) {
	bus := c.Bus()
	if bus == nil {
		return nil, ErrNotConfigured
	}
	return bus.Subscribe(ctx, topic, handler)
}

func (c *Capability) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	bus := c.bus
	ownsBus := c.ownsBus
	registeredDefault := c.registeredDefault
	c.bus = nil
	c.driver = ""
	c.ownsBus = false
	c.registeredDefault = false
	c.mu.Unlock()
	if bus == nil {
		return nil
	}
	if registeredDefault && coreeventbus.Default() == bus {
		if ownsBus {
			return coreeventbus.CloseDefault()
		}
		coreeventbus.SetDefaultWithDriver(nil, "")
		return nil
	}
	if ownsBus {
		return bus.Close()
	}
	return nil
}

func (c *Capability) Capability() coreeventbus.Capability {
	bus := c.Bus()
	if bus == nil {
		return coreeventbus.Capability{}
	}
	return bus.Capability()
}

func (c *Capability) Bus() coreeventbus.Bus {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bus
}

func (c *Capability) Driver() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.driver
}

func attachBus(bus coreeventbus.Bus, driver string, setDefault bool, ownsBus bool) (*Capability, error) {
	if bus == nil {
		return nil, ErrNotConfigured
	}
	driver = normalizeDriver(driver)
	if driver == "" {
		driver = DriverMemory
	}
	if err := ValidateConfig(Config{Driver: driver}); err != nil {
		return nil, err
	}
	if setDefault {
		coreeventbus.SetDefaultWithDriver(bus, driver)
	}
	return &Capability{bus: bus, driver: driver, ownsBus: ownsBus, registeredDefault: setDefault}, nil
}

func buildBus(cfg Config, option options) (coreeventbus.Bus, error) {
	switch cfg.Driver {
	case DriverMemory:
		return eventbusmemory.New(), nil
	case DriverRedis:
		if option.redis == nil {
			return nil, fmt.Errorf("%w: driver=%s", ErrRedisClientRequired, DriverRedis)
		}
		return eventbusredis.New(option.redis, cfg.TopicPrefix), nil
	case DriverRedisStream:
		if option.redis == nil {
			return nil, fmt.Errorf("%w: driver=%s", ErrRedisClientRequired, DriverRedisStream)
		}
		nodeName := eventBusNodeName(cfg)
		return eventbusstream.New(option.redis, cfg.TopicPrefix, nodeName, nodeName, eventbusstream.WithMaxLen(cfg.StreamMaxLen)), nil
	case DriverNATS:
		subjectPrefix := cfg.SubjectPrefix
		if subjectPrefix == "" {
			subjectPrefix = strings.ReplaceAll(cfg.TopicPrefix, ":", ".")
		}
		return eventbusnats.New(cfg.Addr, subjectPrefix)
	default:
		return nil, fmt.Errorf("eventbus driver %q is invalid, want memory, redis, redis_stream, or nats", cfg.Driver)
	}
}

func eventBusNodeName(cfg Config) string {
	if cfg.NodeName != "" {
		return cfg.NodeName
	}
	hostname, err := os.Hostname()
	if err == nil && hostname != "" {
		return hostname
	}
	return "unknown"
}

func ensureDriver(requested string, actual string) error {
	requested = normalizeDriver(requested)
	actual = normalizeDriver(actual)
	if requested == "" || requested == actual {
		return nil
	}
	return fmt.Errorf("eventbus driver mismatch: configured %s, default %s", requested, actual)
}

func normalizeDriver(driver string) string {
	return strings.ToLower(strings.TrimSpace(driver))
}

var _ Service = (*Capability)(nil)
