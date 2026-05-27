//go:build sdkit_eventbus_redis_stream

package eventbus

import (
	"fmt"

	coreeventbus "github.com/huwenlong92/sdkit/core/eventbus"
	"github.com/huwenlong92/sdkit/pkg/eventbus/redisstream"
)

func init() {
	registerDriverFactory(DriverRedisStream, func(cfg Config, option options) (coreeventbus.Bus, error) {
		if option.redis == nil {
			return nil, fmt.Errorf("%w: driver=%s", ErrRedisClientRequired, DriverRedisStream)
		}
		nodeName := eventBusNodeName(cfg)
		return redisstream.New(option.redis, cfg.TopicPrefix, nodeName, nodeName, redisstream.WithMaxLen(cfg.StreamMaxLen)), nil
	})
}
