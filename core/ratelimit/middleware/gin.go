package middleware

import (
	"net/http"

	"github.com/huwenlong92/sdkit/core/errors"
	"github.com/huwenlong92/sdkit/core/ginresponder"
	"github.com/huwenlong92/sdkit/core/ratelimit/keyer"
	"github.com/huwenlong92/sdkit/pkg/ratelimit"

	"github.com/gin-gonic/gin"
)

// Middleware 用 Limiter 对每个 IP 限流
func Middleware(l ratelimit.Limiter) gin.HandlerFunc {
	return MiddlewareWithKey(l, keyer.IP)
}

func MiddlewareWithOptions(l ratelimit.Limiter, opts ...MiddlewareOption) gin.HandlerFunc {
	return MiddlewareWithKeyOptions(l, keyer.IP, opts...)
}

// MiddlewareWithKey 自定义 key 限流（如按用户ID）
func MiddlewareWithKey(l ratelimit.Limiter, keyFn func(*gin.Context) string) gin.HandlerFunc {
	return MiddlewareWithKeyOptions(l, keyFn)
}

func MiddlewareWithKeyOptions(l ratelimit.Limiter, keyFn func(*gin.Context) string, opts ...MiddlewareOption) gin.HandlerFunc {
	cfg := newMiddlewareConfig(opts...)
	return func(c *gin.Context) {
		key := keyFn(c)
		var allowed bool
		if ctxLimiter, ok := l.(ratelimit.ContextLimiter); ok {
			allowed = ctxLimiter.AllowContext(c.Request.Context(), key)
		} else {
			allowed = l.Allow(key)
		}
		if !allowed {
			ginresponder.RespondError(cfg.Responder, c, http.StatusTooManyRequests, errors.NewCodeWithData(http.StatusTooManyRequests, "请求太频繁，请稍后再试", nil))
			return
		}
		c.Next()
	}
}
