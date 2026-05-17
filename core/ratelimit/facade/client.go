package ratelimit

import (
	rlMiddleware "github.com/huwenlong92/sdkit/core/ratelimit/middleware"
	"github.com/huwenlong92/sdkit/core/ratelimit/store"

	goredis "github.com/redis/go-redis/v9"
)

func SetStore(s Store) {
	rlMiddleware.SetStore(s)
}

func NewMemoryStore() Store {
	return store.NewMemoryStore()
}

func NewRedisStore(client *goredis.Client) Store {
	return store.NewRedisStore(client)
}

func NewRedisStoreWithPrefix(client *goredis.Client, prefix string) Store {
	return store.NewRedisStoreWithPrefix(client, prefix)
}
