package middleware

import (
	"net/http"

	"github.com/huwenlong92/sdkit/core/errors"
	ginkeyer "github.com/huwenlong92/sdkit/core/gin/ratelimit/keyer"
	ginresponder "github.com/huwenlong92/sdkit/core/gin/responder"
	coreratelimit "github.com/huwenlong92/sdkit/core/ratelimit"
	"github.com/huwenlong92/sdkit/pkg/ratelimit/strategy"

	"github.com/gin-gonic/gin"
)

// LimiterPerUser 按调用方写入的 subject key 令牌桶限流。
func LimiterPerUser(r float64, burst int) gin.HandlerFunc {
	return LimiterPerUserWithOptions(r, burst)
}

func LimiterPerUserWithOptions(r float64, burst int, opts ...MiddlewareOption) gin.HandlerFunc {
	cfg := newMiddlewareConfig(opts...)
	tb := strategy.NewTokenBucket(r, burst, coreratelimit.PickStore())
	return func(c *gin.Context) {
		key := ginkeyer.User(c)
		if key == "" {
			c.Next()
			return
		}
		if !tb.Allow(key) {
			ginresponder.RespondError(cfg.Responder, c, http.StatusTooManyRequests, errors.NewCodeWithData(http.StatusTooManyRequests, "请求太频繁，请稍后再试", nil))
			return
		}
		c.Next()
	}
}

// LimiterPerUserNormal 每用户 100/s，突发 200
func LimiterPerUserNormal() gin.HandlerFunc { return LimiterPerUser(100, 200) }

// LimiterPerUserStrict 每用户 30/s，突发 60
func LimiterPerUserStrict() gin.HandlerFunc { return LimiterPerUser(30, 60) }

// LimiterPerUserWrite 写操作：每用户 10/s（防刷数据）
func LimiterPerUserWrite() gin.HandlerFunc { return LimiterPerUser(10, 20) }

// LimiterPerUserRoute 按「subject + 路由」限流。
func LimiterPerUserRoute(r float64, burst int) gin.HandlerFunc {
	return LimiterPerUserRouteWithOptions(r, burst)
}

func LimiterPerUserRouteWithOptions(r float64, burst int, opts ...MiddlewareOption) gin.HandlerFunc {
	cfg := newMiddlewareConfig(opts...)
	tb := strategy.NewTokenBucket(r, burst, coreratelimit.PickStore())
	return func(c *gin.Context) {
		key := ginkeyer.UserRoute(c)
		if key == "" {
			c.Next()
			return
		}
		if !tb.Allow(key) {
			ginresponder.RespondError(cfg.Responder, c, http.StatusTooManyRequests, errors.NewCodeWithData(http.StatusTooManyRequests, "请求太频繁，请稍后再试", nil))
			return
		}
		c.Next()
	}
}
