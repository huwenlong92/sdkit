// Package session 提供用户会话管理（Cookie + Store 模式）
package session

import (
	"context"
	"sync"
	"time"

	"github.com/huwenlong92/sdkit/pkg/sessionx"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	Prefix string `mapstructure:"prefix" yaml:"prefix"`
}

// Session 是 core/session 对外会话主体。
type Session = sessionx.Session

// Store 是 core/session 对外会话存储接口。
type Store = sessionx.Store

type storeContextKey struct{}

// 存储构造函数。
var (
	NewMemoryStore           = sessionx.NewMemoryStore
	NewMemoryStoreWithPrefix = sessionx.NewMemoryStoreWithPrefix
	NewRedisStore            = sessionx.NewRedisStore
	NewRedisStoreWithPrefix  = sessionx.NewRedisStoreWithPrefix
)

// SessionTTL 会话默认有效期
const SessionTTL = 30 * time.Minute

var (
	globalStore Store
	once        sync.Once
)

// Init 初始化全局会话存储（须在 bootstrap 中调用）
func Init(rdb *redis.Client, cfg *Config) {
	once.Do(func() {
		globalStore = NewStore(rdb, cfg)
	})
}

func NewStore(rdb *redis.Client, cfg *Config) Store {
	prefix := "session:"
	if cfg != nil && cfg.Prefix != "" {
		prefix = cfg.Prefix
	}
	if rdb != nil {
		return sessionx.NewRedisStoreWithPrefix(rdb, prefix)
	}
	return sessionx.NewMemoryStoreWithPrefix(prefix)
}

// GetStore 返回全局会话存储
func GetStore() Store {
	if globalStore == nil {
		panic("session store not initialized")
	}
	return globalStore
}

func ContextWithStore(ctx context.Context, store Store) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if store == nil {
		return ctx
	}
	return context.WithValue(ctx, storeContextKey{}, store)
}

func StoreFromContext(ctx context.Context) (Store, bool) {
	if ctx == nil {
		return nil, false
	}
	store, ok := ctx.Value(storeContextKey{}).(Store)
	if !ok || store == nil {
		return nil, false
	}
	return store, true
}

func FromContext(ctx context.Context) (Store, bool) {
	return StoreFromContext(ctx)
}
