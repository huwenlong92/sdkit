package middleware

import (
	ginkeyer "github.com/huwenlong92/sdkit/core/gin/ratelimit/keyer"
	ginresponder "github.com/huwenlong92/sdkit/core/gin/responder"
	coreratelimit "github.com/huwenlong92/sdkit/core/ratelimit"
	"github.com/huwenlong92/sdkit/pkg/ratelimit"
	"github.com/huwenlong92/sdkit/pkg/ratelimit/strategy"

	"github.com/gin-gonic/gin"
)

type MiddlewareConfig struct {
	Responder ginresponder.ErrorResponder
}

type MiddlewareOption func(*MiddlewareConfig)

func WithResponder(responder ginresponder.ErrorResponder) MiddlewareOption {
	return func(cfg *MiddlewareConfig) {
		cfg.Responder = responder
	}
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
	return MiddlewareWithKeyOptions(strategy.NewTokenBucket(r, burst, coreratelimit.PickStore()), ginkeyer.IP, opts...)
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
