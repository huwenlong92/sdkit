package queue

import "time"

const DefaultQueueName = "default"

const defaultConcurrency = 10
const defaultRateLimitWindow = time.Minute
const defaultNATSStream = "SDKT_QUEUE"
const defaultNATSSubjectPrefix = "sdkit.queue"
const defaultNATSDurablePrefix = "sdkit"

type RedisConfig struct {
	Addr     string `mapstructure:"addr" yaml:"addr"`
	Password string `mapstructure:"password" yaml:"password"`
	DB       int    `mapstructure:"db" yaml:"db"`
}

type NATSConfig struct {
	Stream        string        `mapstructure:"stream" yaml:"stream"`
	SubjectPrefix string        `mapstructure:"subject_prefix" yaml:"subject_prefix"`
	DurablePrefix string        `mapstructure:"durable_prefix" yaml:"durable_prefix"`
	AckWait       time.Duration `mapstructure:"ack_wait" yaml:"ack_wait"`
	MaxDeliver    int           `mapstructure:"max_deliver" yaml:"max_deliver"`
	MaxAge        time.Duration `mapstructure:"max_age" yaml:"max_age"`
	Duplicates    time.Duration `mapstructure:"duplicates" yaml:"duplicates"`
	Storage       string        `mapstructure:"storage" yaml:"storage"`
	Replicas      int           `mapstructure:"replicas" yaml:"replicas"`
	FetchBatch    int           `mapstructure:"fetch_batch" yaml:"fetch_batch"`
	FetchWait     time.Duration `mapstructure:"fetch_wait" yaml:"fetch_wait"`
	RetryDelay    time.Duration `mapstructure:"retry_delay" yaml:"retry_delay"`
}

type RateLimitConfig struct {
	Enabled       bool          `mapstructure:"enabled" yaml:"enabled"`
	DefaultLimit  int           `mapstructure:"default_limit" yaml:"default_limit"`
	DefaultWindow time.Duration `mapstructure:"default_window" yaml:"default_window"`
}

type LockConfig struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled"`
	Prefix  string `mapstructure:"prefix" yaml:"prefix"`
}

type IdempotencyConfig struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled"`
	Prefix  string `mapstructure:"prefix" yaml:"prefix"`
}

type OutboxConfig struct {
	Enabled       bool          `mapstructure:"enabled" yaml:"enabled"`
	FlushInterval time.Duration `mapstructure:"flush_interval" yaml:"flush_interval"`
	BatchSize     int           `mapstructure:"batch_size" yaml:"batch_size"`
}

type ScheduleConfig struct {
	Enabled      bool          `mapstructure:"enabled" yaml:"enabled"`
	PollInterval time.Duration `mapstructure:"poll_interval" yaml:"poll_interval"`
	BatchSize    int           `mapstructure:"batch_size" yaml:"batch_size"`
}

type WorkerProfile struct {
	Name            string         `mapstructure:"name" yaml:"name"`
	Concurrency     int            `mapstructure:"concurrency" yaml:"concurrency"`
	Queues          map[string]int `mapstructure:"queues" yaml:"queues"`
	StrictPriority  bool           `mapstructure:"strict_priority" yaml:"strict_priority"`
	ShutdownTimeout time.Duration  `mapstructure:"shutdown_timeout" yaml:"shutdown_timeout"`
}

type Config struct {
	Driver string      `mapstructure:"driver" yaml:"driver"`
	Redis  RedisConfig `mapstructure:"redis" yaml:"redis"`
	NATS   NATSConfig  `mapstructure:"nats" yaml:"nats"`

	Addr     string `mapstructure:"addr" yaml:"addr"`
	Password string `mapstructure:"password" yaml:"password"`
	DB       int    `mapstructure:"db" yaml:"db"`

	Concurrency    int            `mapstructure:"concurrency" yaml:"concurrency"`
	Queues         map[string]int `mapstructure:"queues" yaml:"queues"`
	StrictPriority bool           `mapstructure:"strict_priority" yaml:"strict_priority"`

	Workers     map[string]WorkerProfile `mapstructure:"workers" yaml:"workers"`
	RateLimit   RateLimitConfig          `mapstructure:"rate_limit" yaml:"rate_limit"`
	Lock        LockConfig               `mapstructure:"lock" yaml:"lock"`
	Idempotency IdempotencyConfig        `mapstructure:"idempotency" yaml:"idempotency"`
	Outbox      OutboxConfig             `mapstructure:"outbox" yaml:"outbox"`
	Schedule    ScheduleConfig           `mapstructure:"schedule" yaml:"schedule"`
}

func FromConfig(cfg *Config) Config {
	if cfg == nil {
		return Config{}
	}
	return *cfg
}

func (c Config) Normalize() Config {
	if c.Driver == "" {
		c.Driver = "asynq"
	}
	if c.Redis.Addr == "" && c.Addr != "" {
		c.Redis.Addr = c.Addr
		c.Redis.Password = c.Password
		c.Redis.DB = c.DB
	}
	if c.Addr == "" && c.Redis.Addr != "" {
		c.Addr = c.Redis.Addr
		c.Password = c.Redis.Password
		c.DB = c.Redis.DB
	}
	if c.Concurrency <= 0 {
		c.Concurrency = defaultConcurrency
	}
	if len(c.Queues) == 0 {
		c.Queues = map[string]int{DefaultQueueName: 1}
	}
	if c.NATS.Stream == "" {
		c.NATS.Stream = defaultNATSStream
	}
	if c.NATS.SubjectPrefix == "" {
		c.NATS.SubjectPrefix = defaultNATSSubjectPrefix
	}
	if c.NATS.DurablePrefix == "" {
		c.NATS.DurablePrefix = defaultNATSDurablePrefix
	}
	if c.NATS.AckWait <= 0 {
		c.NATS.AckWait = 30 * time.Second
	}
	if c.NATS.MaxDeliver <= 0 {
		c.NATS.MaxDeliver = 5
	}
	if c.NATS.Duplicates <= 0 {
		c.NATS.Duplicates = 2 * time.Minute
	}
	if c.NATS.Storage == "" {
		c.NATS.Storage = "file"
	}
	if c.NATS.Replicas <= 0 {
		c.NATS.Replicas = 1
	}
	if c.NATS.FetchBatch <= 0 {
		c.NATS.FetchBatch = c.Concurrency
	}
	if c.NATS.FetchWait <= 0 {
		c.NATS.FetchWait = time.Second
	}
	if c.NATS.RetryDelay <= 0 {
		c.NATS.RetryDelay = 5 * time.Second
	}
	if c.Schedule.PollInterval <= 0 {
		c.Schedule.PollInterval = time.Second
	}
	if c.Schedule.BatchSize <= 0 {
		c.Schedule.BatchSize = 50
	}
	return c
}

func (c Config) WorkerProfile(name string) WorkerProfile {
	c = c.Normalize()
	if name != "" {
		if profile, ok := c.Workers[name]; ok {
			return profile.Normalize(name, c)
		}
	}
	return WorkerProfile{
		Name:            name,
		Concurrency:     c.Concurrency,
		Queues:          c.Queues,
		StrictPriority:  c.StrictPriority,
		ShutdownTimeout: 30 * time.Second,
	}.Normalize(name, c)
}

func (p WorkerProfile) Normalize(name string, cfg Config) WorkerProfile {
	if p.Name == "" {
		p.Name = name
	}
	if p.Concurrency <= 0 {
		if cfg.Concurrency > 0 {
			p.Concurrency = cfg.Concurrency
		} else {
			p.Concurrency = defaultConcurrency
		}
	}
	if len(p.Queues) == 0 {
		if len(cfg.Queues) > 0 {
			p.Queues = cfg.Queues
		} else {
			p.Queues = map[string]int{DefaultQueueName: 1}
		}
	}
	if p.ShutdownTimeout <= 0 {
		p.ShutdownTimeout = 30 * time.Second
	}
	return p
}

func normalizeConfig(c Config) Config {
	return c.Normalize()
}
