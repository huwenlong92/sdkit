package ratelimit

type Config struct {
	Rate  float64 `mapstructure:"rate" yaml:"rate"`
	Burst int     `mapstructure:"burst" yaml:"burst"`
}

type BBRConfig struct {
	Enabled      bool    `mapstructure:"enabled" yaml:"enabled"`
	CPUThreshold int64   `mapstructure:"cpu_threshold" yaml:"cpu_threshold"`
	Window       int     `mapstructure:"window" yaml:"window"`
	Decay        float64 `mapstructure:"decay" yaml:"decay"`
}
