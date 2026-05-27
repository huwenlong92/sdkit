//go:build sdkit_eventbus_redis

package eventbus

import (
	"fmt"

	coreeventbus "github.com/huwenlong92/sdkit/core/eventbus"
	eventbusredis "github.com/huwenlong92/sdkit/pkg/eventbus/redis"
)

func init() {
	registerDriverFactory(DriverRedis, func(cfg Config, option options) (coreeventbus.Bus, error) {
		if option.redis == nil {
			return nil, fmt.Errorf("%w: driver=%s", ErrRedisClientRequired, DriverRedis)
		}
		return eventbusredis.New(option.redis, cfg.TopicPrefix), nil
	})
}
