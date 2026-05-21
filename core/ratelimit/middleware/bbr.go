package middleware

import (
	"net/http"
	"time"

	apperrors "github.com/huwenlong92/sdkit/core/errors"
	"github.com/huwenlong92/sdkit/core/ginresponder"
	"github.com/huwenlong92/sdkit/pkg/ratelimit/strategy"

	"github.com/gin-gonic/gin"
)

type BBRMiddlewareConfig struct {
	Responder ginresponder.ErrorResponder
}

type BBRMiddlewareOption func(*BBRMiddlewareConfig)

func WithBBRResponder(responder ginresponder.ErrorResponder) BBRMiddlewareOption {
	return func(cfg *BBRMiddlewareConfig) {
		cfg.Responder = responder
	}
}

func BBRMiddleware(l *strategy.BBR) gin.HandlerFunc {
	return BBRMiddlewareWithOptions(l)
}

func BBRMiddlewareWithOptions(l *strategy.BBR, opts ...BBRMiddlewareOption) gin.HandlerFunc {
	cfg := newBBRMiddlewareConfig(opts...)
	return func(c *gin.Context) {
		done, err := l.Allow()
		if err != nil {
			ginresponder.RespondError(cfg.Responder, c, http.StatusTooManyRequests, apperrors.NewCodeWithData(http.StatusTooManyRequests, "服务繁忙，请稍后再试", nil))
			return
		}
		c.Next()
		done()
	}
}

// BBRNormal 常规 BBR：CPU 80% 触发限流，1s 采样间隔
func BBRNormal() gin.HandlerFunc {
	return BBRMiddleware(strategy.NewBBR(
		strategy.WithCPUThreshold(800),
		strategy.WithWindow(time.Second),
		strategy.WithDecay(0.95),
	))
}

// BBRSensitive 敏感 BBR：CPU 60% 触发限流，更快响应
func BBRSensitive() gin.HandlerFunc {
	return BBRMiddleware(strategy.NewBBR(
		strategy.WithCPUThreshold(600),
		strategy.WithWindow(500*time.Millisecond),
		strategy.WithDecay(0.9),
	))
}

func newBBRMiddlewareConfig(opts ...BBRMiddlewareOption) *BBRMiddlewareConfig {
	cfg := &BBRMiddlewareConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}
