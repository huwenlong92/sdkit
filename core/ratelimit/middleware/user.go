package middleware

import (
	"github.com/huwenlong92/sdkit/core/ratelimit/keyer"
	"github.com/huwenlong92/sdkit/core/ratelimit/strategy"
	"github.com/huwenlong92/sdkit/core/response"

	"github.com/gin-gonic/gin"
)

// LimiterPerUser 按用户 ID 令牌桶限流（须在 JWTAuth 之后注册）
func LimiterPerUser(r float64, burst int) gin.HandlerFunc {
	tb := strategy.NewTokenBucket(r, burst, pickStore())
	return func(c *gin.Context) {
		key := keyer.User(c)
		if key == "" {
			c.Next()
			return
		}
		if !tb.Allow(key) {
			response.AbortJSON(c, 429, gin.H{"err_code": 429, "msg": "请求太频繁，请稍后再试"})
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

// LimiterPerUserRoute 按「用户 + 路由」限流（须在 JWTAuth 之后注册）
func LimiterPerUserRoute(r float64, burst int) gin.HandlerFunc {
	tb := strategy.NewTokenBucket(r, burst, pickStore())
	return func(c *gin.Context) {
		key := keyer.UserRoute(c)
		if key == "" {
			c.Next()
			return
		}
		if !tb.Allow(key) {
			response.AbortJSON(c, 429, gin.H{"err_code": 429, "msg": "请求太频繁，请稍后再试"})
			return
		}
		c.Next()
	}
}
