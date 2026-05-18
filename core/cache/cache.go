// Package cache 提供缓存抽象入口，默认支持内存和 Redis。
//
// 全局默认（bootstrap 自动 Init）：
//
//	c := cache.Default()
//
// 服务自定义：
//
//	c := cache.New(cache.WithRedis(client), cache.WithPrefix("svc:"))
package cache

import (
	"github.com/huwenlong92/sdkit/core/logger"
	coreredis "github.com/huwenlong92/sdkit/core/redis"
	pkgcache "github.com/huwenlong92/sdkit/pkg/cache"
	"github.com/huwenlong92/sdkit/pkg/cache/memory"
	rediscache "github.com/huwenlong92/sdkit/pkg/cache/redis"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Cache 是 core/cache 对外缓存接口，底层由 pkg/cache 后端实现。
type Cache = pkgcache.Cache

var defaultCache Cache

type Config struct {
	Prefix string `mapstructure:"prefix" yaml:"prefix"`
}

// Default 返回全局默认缓存（Init 之后为 Redis，否则内存）
func Default() Cache {
	if defaultCache != nil {
		return defaultCache
	}
	return New()
}

// ======================== Init（bootstrap 统一入口） ========================

// Init 初始化全局缓存：有 Redis 用 Redis，否则内存
func Init(cacheCfg *Config) error {
	log := logger.L
	if log == nil {
		log = zap.NewNop()
	}

	prefix := "cache:"
	if cacheCfg != nil && cacheCfg.Prefix != "" {
		prefix = cacheCfg.Prefix
	}

	if coreredis.RDB != nil {
		defaultCache = rediscache.New(coreredis.RDB, prefix)
	} else {
		log.Debug("Redis未初始化，使用内存缓存")
		defaultCache = memory.New()
	}
	return nil
}

// Close 关闭全局缓存
func Close() {
	if defaultCache != nil {
		_ = defaultCache.Close()
		defaultCache = nil
	}
}

// ======================== 服务自定义 ========================

// Option 缓存配置选项
type Option func(*options)

type options struct {
	redis  *redis.Client
	prefix string
}

// WithRedis 使用 Redis 后端
func WithRedis(client *redis.Client) Option {
	return func(o *options) { o.redis = client }
}

// WithPrefix 设置 key 前缀
func WithPrefix(prefix string) Option {
	return func(o *options) { o.prefix = prefix }
}

// New 创建缓存实例（服务自定义配置）
func New(opts ...Option) Cache {
	o := options{prefix: "cache:"}
	for _, opt := range opts {
		opt(&o)
	}
	if o.redis != nil {
		return rediscache.New(o.redis, o.prefix)
	}
	return memory.New()
}
