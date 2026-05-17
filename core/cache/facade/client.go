package cache

import (
	corecache "github.com/huwenlong92/sdkit/core/cache"

	goredis "github.com/redis/go-redis/v9"
)

func Init(cacheCfg *Config) error {
	return corecache.Init(cacheCfg)
}

func Close() {
	corecache.Close()
}

func New(opts ...Option) Cache {
	return corecache.New(opts...)
}

func WithRedis(client *goredis.Client) Option {
	return corecache.WithRedis(client)
}

func WithPrefix(prefix string) Option {
	return corecache.WithPrefix(prefix)
}
