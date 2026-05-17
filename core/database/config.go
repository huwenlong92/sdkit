package database

import (
	"errors"
	"time"
)

const (
	defaultDriver          = "postgres"
	defaultMaxOpenConns    = 25
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = 3600
)

type Config struct {
	Driver          string `mapstructure:"driver" yaml:"driver"`
	DSN             string `mapstructure:"dsn" yaml:"dsn"`
	TablePrefix     string `mapstructure:"table_prefix" yaml:"table_prefix"`
	Schema          string `mapstructure:"schema" yaml:"schema"`
	MaxOpenConns    int    `mapstructure:"max_open_conns" yaml:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns" yaml:"max_idle_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime" yaml:"conn_max_lifetime"`
	LogLevel        string `mapstructure:"log_level" yaml:"log_level"`
}

func fromAppConfig(cfg *Config) Config {
	if cfg == nil {
		return Config{}
	}
	return *cfg
}

func normalizeConfig(cfg Config) (Config, error) {
	if cfg.Driver == "" {
		cfg.Driver = defaultDriver
	}
	if cfg.DSN == "" {
		return cfg, errors.New("database dsn is required")
	}
	if cfg.MaxOpenConns <= 0 {
		cfg.MaxOpenConns = defaultMaxOpenConns
	}
	if cfg.MaxIdleConns <= 0 {
		cfg.MaxIdleConns = defaultMaxIdleConns
	}
	if cfg.MaxIdleConns > cfg.MaxOpenConns {
		cfg.MaxIdleConns = cfg.MaxOpenConns
	}
	if cfg.ConnMaxLifetime <= 0 {
		cfg.ConnMaxLifetime = defaultConnMaxLifetime
	}
	return cfg, nil
}

func (cfg Config) connMaxLifetime() time.Duration {
	return time.Duration(cfg.ConnMaxLifetime) * time.Second
}
