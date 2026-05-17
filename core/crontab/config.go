package crontab

import "time"

type Config struct {
	Enabled        bool          `mapstructure:"enabled" yaml:"enabled"`
	Driver         string        `mapstructure:"driver" yaml:"driver"` // robfig | asynq
	ReloadInterval time.Duration `mapstructure:"reload_interval" yaml:"reload_interval"`
	InstanceID     string        `mapstructure:"instance_id" yaml:"instance_id"`

	Lock LockConfig `mapstructure:"lock" yaml:"lock"`
	Log  LogConfig  `mapstructure:"log" yaml:"log"`
}

type LockConfig struct {
	Enabled bool          `mapstructure:"enabled" yaml:"enabled"`
	TTL     time.Duration `mapstructure:"ttl" yaml:"ttl"`
}

type LogConfig struct {
	Enabled       bool          `mapstructure:"enabled" yaml:"enabled"`
	Batch         bool          `mapstructure:"batch" yaml:"batch"`
	BatchSize     int           `mapstructure:"batch_size" yaml:"batch_size"`
	FlushInterval time.Duration `mapstructure:"flush_interval" yaml:"flush_interval"`
}

func DefaultConfig() Config {
	return Config{
		Enabled:        true,
		Driver:         "robfig",
		ReloadInterval: 30 * time.Second,
		Lock: LockConfig{
			Enabled: true,
			TTL:     10 * time.Minute,
		},
		Log: LogConfig{
			Enabled:       true,
			Batch:         true,
			BatchSize:     100,
			FlushInterval: 3 * time.Second,
		},
	}
}
