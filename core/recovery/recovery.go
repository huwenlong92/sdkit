package recovery

import (
	"net"
	"net/http"
	"os"
	"runtime/debug"

	apperrors "github.com/huwenlong92/sdkit/core/errors"
	"github.com/huwenlong92/sdkit/core/ginresponder"
	"github.com/huwenlong92/sdkit/core/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
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

func Middleware(opts ...MiddlewareOption) gin.HandlerFunc {
	cfg := newMiddlewareConfig(opts...)
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				var brokenPipe bool
				if ne, ok := err.(*net.OpError); ok {
					if se, ok := ne.Err.(*os.SyscallError); ok {
						if se.Error() == "broken pipe" || se.Error() == "connection reset by peer" {
							brokenPipe = true
						}
					}
				}

				if brokenPipe {
					c.Error(err.(error))
					c.Abort()
					return
				}

				logger.L.Error("panic recovered",
					zap.Any("error", err),
					zap.String("request", c.Request.Method+" "+c.Request.URL.String()),
					zap.String("stack", string(debug.Stack())),
				)

				ginresponder.RespondError(cfg.Responder, c, http.StatusInternalServerError, apperrors.NewCodeWithData(apperrors.CodeInternal, "服务器内部错误", nil))
			}
		}()
		c.Next()
	}
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
