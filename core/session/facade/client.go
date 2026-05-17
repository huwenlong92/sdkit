package session

import (
	coresession "github.com/huwenlong92/sdkit/core/session"

	goredis "github.com/redis/go-redis/v9"
)

func Init(rdb *goredis.Client, cfg *Config) {
	coresession.Init(rdb, cfg)
}

func NewMemoryStore() Store {
	return coresession.NewMemoryStore()
}

func NewMemoryStoreWithPrefix(prefix string) Store {
	return coresession.NewMemoryStoreWithPrefix(prefix)
}

func NewRedisStore(rdb *goredis.Client) Store {
	return coresession.NewRedisStore(rdb)
}

func NewRedisStoreWithPrefix(rdb *goredis.Client, prefix string) Store {
	return coresession.NewRedisStoreWithPrefix(rdb, prefix)
}
