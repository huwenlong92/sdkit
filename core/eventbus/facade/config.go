package eventbus

import (
	coreeventbus "github.com/huwenlong92/sdkit/core/eventbus"
	"github.com/huwenlong92/sdkit/core/runtime"
)

const (
	Name        = "eventbus"
	KeyEventBus = coreeventbus.KeyEventBus

	DriverMemory      = "memory"
	DriverRedis       = "redis"
	DriverRedisStream = "redis_stream"
)

type Config struct {
	Driver       string `mapstructure:"driver" yaml:"driver"`
	TopicPrefix  string `mapstructure:"topic_prefix" yaml:"topic_prefix"`
	NodeName     string `mapstructure:"node_name" yaml:"node_name"`
	StreamMaxLen int64  `mapstructure:"stream_max_len" yaml:"stream_max_len"`
}

type ConfigLoader func(app *runtime.App) (Config, error)
