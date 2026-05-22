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
	DriverNATS        = "nats"
)

type Config struct {
	Driver        string `mapstructure:"driver" yaml:"driver"`
	Addr          string `mapstructure:"addr" yaml:"addr"`
	TopicPrefix   string `mapstructure:"topic_prefix" yaml:"topic_prefix"`
	SubjectPrefix string `mapstructure:"subject_prefix" yaml:"subject_prefix"`
	NodeName      string `mapstructure:"node_name" yaml:"node_name"`
	StreamMaxLen  int64  `mapstructure:"stream_max_len" yaml:"stream_max_len"`
}

type ConfigLoader func(app *runtime.App) (Config, error)
