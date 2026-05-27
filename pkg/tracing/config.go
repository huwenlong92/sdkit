package tracing

import "time"

const (
	defaultServiceName = "sdkitgo"
	defaultEndpoint    = "127.0.0.1:4317"
	defaultTimeout     = 5 * time.Second
)

type Config struct {
	Enabled     bool          `mapstructure:"enabled" yaml:"enabled"`
	ServiceName string        `mapstructure:"service_name" yaml:"service_name"`
	Environment string        `mapstructure:"environment" yaml:"environment"`
	Endpoint    string        `mapstructure:"endpoint" yaml:"endpoint"`
	Insecure    bool          `mapstructure:"insecure" yaml:"insecure"`
	SampleRatio float64       `mapstructure:"sample_ratio" yaml:"sample_ratio"`
	Strict      bool          `mapstructure:"strict" yaml:"strict"`
	Timeout     time.Duration `mapstructure:"timeout" yaml:"timeout"`
}

func DefaultConfig() Config {
	return Config{
		ServiceName: defaultServiceName,
		Endpoint:    defaultEndpoint,
		Insecure:    true,
		SampleRatio: 1,
		Timeout:     defaultTimeout,
	}
}

func NormalizeConfig(cfg Config) Config {
	defaults := DefaultConfig()
	if cfg.ServiceName == "" {
		cfg.ServiceName = defaults.ServiceName
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = defaults.Endpoint
	}
	if cfg.SampleRatio < 0 {
		cfg.SampleRatio = 0
	}
	if cfg.SampleRatio > 1 {
		cfg.SampleRatio = 1
	}
	if cfg.SampleRatio == 0 {
		cfg.SampleRatio = defaults.SampleRatio
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaults.Timeout
	}
	return cfg
}
