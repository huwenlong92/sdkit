package middleware

import (
	"time"

	"github.com/huwenlong92/sdkit/core/ratelimit/strategy"

	"github.com/gin-gonic/gin"
)

// BBRNormal 常规 BBR：CPU 80% 触发限流，1s 采样间隔
func BBRNormal() gin.HandlerFunc {
	return strategy.BBRMiddleware(strategy.NewBBR(
		strategy.WithCPUThreshold(800),
		strategy.WithWindow(time.Second),
		strategy.WithDecay(0.95),
	))
}

// BBRSensitive 敏感 BBR：CPU 60% 触发限流，更快响应
func BBRSensitive() gin.HandlerFunc {
	return strategy.BBRMiddleware(strategy.NewBBR(
		strategy.WithCPUThreshold(600),
		strategy.WithWindow(500*time.Millisecond),
		strategy.WithDecay(0.9),
	))
}
