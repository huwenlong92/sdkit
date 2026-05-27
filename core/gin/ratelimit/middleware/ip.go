package middleware

import (
	"time"

	coreratelimit "github.com/huwenlong92/sdkit/core/ratelimit"
	"github.com/huwenlong92/sdkit/pkg/ratelimit/strategy"

	"github.com/gin-gonic/gin"
)

// LimiterLoose 宽松：每 IP 200/s，突发 400
func LimiterLoose() gin.HandlerFunc { return Limiter(200, 400) }

// LimiterNormal 常规：每 IP 100/s，突发 200
func LimiterNormal() gin.HandlerFunc { return Limiter(100, 200) }

// LimiterStrict 严格：每 IP 20/s，突发 50
func LimiterStrict() gin.HandlerFunc { return Limiter(20, 50) }

// LimiterLogin 登录保护：每 IP 每分钟 5 次（防暴力破解）
func LimiterLogin() gin.HandlerFunc {
	return Middleware(strategy.NewSlidingWindow(time.Minute, 5, coreratelimit.PickStore()))
}

// LimiterUpload 上传接口：每 IP 10/s，突发 30
func LimiterUpload() gin.HandlerFunc { return Limiter(10, 30) }

// LimiterLeaky 漏桶限流：rate=漏出速率/秒，capacity=桶容量
func LimiterLeaky(rate float64, capacity int) gin.HandlerFunc {
	return Middleware(strategy.NewLeakyBucket(rate, capacity, coreratelimit.PickStore()))
}
