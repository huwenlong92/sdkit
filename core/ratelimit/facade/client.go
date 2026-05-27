package ratelimit

import (
	coreratelimit "github.com/huwenlong92/sdkit/core/ratelimit"
	"github.com/huwenlong92/sdkit/pkg/ratelimit/store"

	goredis "github.com/redis/go-redis/v9"
)

func SetStore(s Store) {
	coreratelimit.SetStore(s)
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
