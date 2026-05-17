package middleware

import (
	"net/http"

	"github.com/huwenlong92/sdkit/core/ratelimit"
	"github.com/huwenlong92/sdkit/core/ratelimit/keyer"
	"github.com/huwenlong92/sdkit/core/response"

	"github.com/gin-gonic/gin"
)

// Middleware 用 Limiter 对每个 IP 限流
func Middleware(l ratelimit.Limiter) gin.HandlerFunc {
	return MiddlewareWithKey(l, keyer.IP)
}

// MiddlewareWithKey 自定义 key 限流（如按用户ID）
func MiddlewareWithKey(l ratelimit.Limiter, keyFn func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := keyFn(c)
		var allowed bool
		if ctxLimiter, ok := l.(ratelimit.ContextLimiter); ok {
			allowed = ctxLimiter.AllowContext(c.Request.Context(), key)
		} else {
			allowed = l.Allow(key)
		}
		if !allowed {
			response.AbortJSON(c, http.StatusTooManyRequests, gin.H{
				"err_code": http.StatusTooManyRequests,
				"msg":      "请求太频繁，请稍后再试",
			})
			return
		}
		c.Next()
	}
}
