package strategy

import (
	"net/http"

	apperrors "github.com/huwenlong92/sdkit/core/errors"
	"github.com/huwenlong92/sdkit/core/ginresponder"

	"github.com/gin-gonic/gin"
)

type BBRMiddlewareConfig struct {
	Responder ginresponder.ErrorResponder
}

type BBRMiddlewareOption func(*BBRMiddlewareConfig)

func WithResponder(responder ginresponder.ErrorResponder) BBRMiddlewareOption {
	return func(cfg *BBRMiddlewareConfig) {
		cfg.Responder = responder
	}
}

// BBRMiddleware 返回 BBR Gin 中间件
func BBRMiddleware(l *BBR) gin.HandlerFunc {
	return BBRMiddlewareWithOptions(l)
}

func BBRMiddlewareWithOptions(l *BBR, opts ...BBRMiddlewareOption) gin.HandlerFunc {
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

func newBBRMiddlewareConfig(opts ...BBRMiddlewareOption) *BBRMiddlewareConfig {
	cfg := &BBRMiddlewareConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}
