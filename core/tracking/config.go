package tracking

const (
	DefaultHeader = "X-Track-ID"
	Header        = DefaultHeader
	Key           = "track_id"

	generatorUUID = "uuid"
)

type Config struct {
	Enabled        bool   `mapstructure:"enabled" yaml:"enabled"`
	Header         string `mapstructure:"header" yaml:"header"`
	ResponseHeader string `mapstructure:"response_header" yaml:"response_header"`
	Generator      string `mapstructure:"generator" yaml:"generator"`
	ForceNew       bool   `mapstructure:"force_new" yaml:"force_new"`
}

func DefaultConfig() Config {
	return Config{
		Enabled:        true,
		Header:         DefaultHeader,
		ResponseHeader: DefaultHeader,
		Generator:      generatorUUID,
	}
}

func NormalizeConfig(cfg Config) Config {
	defaults := DefaultConfig()
	if cfg.Header == "" {
		cfg.Header = defaults.Header
	}
	if cfg.ResponseHeader == "" {
		cfg.ResponseHeader = cfg.Header
	}
	if cfg.Generator == "" {
		cfg.Generator = defaults.Generator
	}
	return cfg
}
