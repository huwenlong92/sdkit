package middleware

import (
	"github.com/huwenlong92/sdkit/core/ginresponder"
	"github.com/huwenlong92/sdkit/core/ratelimit/keyer"
	"github.com/huwenlong92/sdkit/pkg/ratelimit"
	"github.com/huwenlong92/sdkit/pkg/ratelimit/store"
	"github.com/huwenlong92/sdkit/pkg/ratelimit/strategy"

	"github.com/gin-gonic/gin"
)

// CustomStore 全局自定义存储，SetStore 注入后所有预设使用同一个 Store（如 Redis）
var CustomStore store.Store

type MiddlewareConfig struct {
	Responder ginresponder.ErrorResponder
}

type MiddlewareOption func(*MiddlewareConfig)

func WithResponder(responder ginresponder.ErrorResponder) MiddlewareOption {
	return func(cfg *MiddlewareConfig) {
		cfg.Responder = responder
	}
}

// SetStore 设置全局限流存储（如 store.NewRedisStore(rdb)），nil 恢复默认内存
func SetStore(s store.Store) { CustomStore = s }

func pickStore() store.Store {
	if CustomStore != nil {
		return CustomStore
	}
	return store.NewMemoryStore()
}

// LimiterStrategy 使用指定 Limiter 接口实现的中间件
func LimiterStrategy(l ratelimit.Limiter) gin.HandlerFunc {
	return Middleware(l)
}

func LimiterStrategyWithOptions(l ratelimit.Limiter, opts ...MiddlewareOption) gin.HandlerFunc {
	return MiddlewareWithOptions(l, opts...)
}

// Limiter 每个 IP 令牌桶限流
func Limiter(r float64, burst int) gin.HandlerFunc {
	return LimiterWithOptions(r, burst)
}

func LimiterWithOptions(r float64, burst int, opts ...MiddlewareOption) gin.HandlerFunc {
	return MiddlewareWithKeyOptions(strategy.NewTokenBucket(r, burst, pickStore()), keyer.IP, opts...)
}

func newMiddlewareConfig(opts ...MiddlewareOption) *MiddlewareConfig {
	cfg := &MiddlewareConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}
