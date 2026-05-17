package realtime

import "time"

type Config struct {
	Topic            string
	ClientBufferSize int
	Logger           Logger
}

type Options struct {
	ClientBufferSize int
	Logger           Logger
}

type Option func(*Options)

func NewOptions(opts ...Option) Options {
	options := Options{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return options.normalize()
}

func WithClientBufferSize(size int) Option {
	return func(o *Options) {
		o.ClientBufferSize = size
	}
}

func WithLogger(log Logger) Option {
	return func(o *Options) {
		o.Logger = log
	}
}

func DefaultConfig() Config {
	return Config{
		Topic:            DefaultTopic,
		ClientBufferSize: 64,
		Logger:           nopLogger{},
	}
}

func (c Config) Options() Options {
	return Options{
		ClientBufferSize: c.ClientBufferSize,
		Logger:           c.Logger,
	}.normalize()
}

func (o Options) normalize() Options {
	if o.ClientBufferSize <= 0 {
		o.ClientBufferSize = 64
	}
	if o.Logger == nil {
		o.Logger = nopLogger{}
	}
	return o
}

type ServerOptions struct {
	Addr              string
	Path              string
	HeartbeatInterval time.Duration
	WriteTimeout      time.Duration
	ClientBufferSize  int
	AllowEvents       []string
	Auth              AuthOptions
	EventBusDriver    string
	EventBusTopic     string
	EventBusPrefix    string
	StreamMaxLen      int64
	NodeName          string
}

type AuthOptions struct {
	Enabled     bool
	TokenQuery  string
	AllowCookie bool
	JWTSecret   string
}
