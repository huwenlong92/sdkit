package middleware

import (
	"github.com/huwenlong92/sdkit/core/ratelimit"
	"github.com/huwenlong92/sdkit/core/ratelimit/keyer"
	"github.com/huwenlong92/sdkit/core/ratelimit/store"
	"github.com/huwenlong92/sdkit/core/ratelimit/strategy"

	"github.com/gin-gonic/gin"
)

// CustomStore 全局自定义存储，SetStore 注入后所有预设使用同一个 Store（如 Redis）
var CustomStore store.Store

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

// Limiter 每个 IP 令牌桶限流
func Limiter(r float64, burst int) gin.HandlerFunc {
	return MiddlewareWithKey(strategy.NewTokenBucket(r, burst, pickStore()), keyer.IP)
}
