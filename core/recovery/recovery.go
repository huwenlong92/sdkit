package recovery

import (
	"net"
	"net/http"
	"os"
	"runtime/debug"

	apperrors "github.com/huwenlong92/sdkit/core/errors"
	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func Middleware() gin.HandlerFunc {
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

				response.Error(c, apperrors.NewCodeWithData(http.StatusInternalServerError, "服务器内部错误", nil))
				c.Abort()
			}
		}()
		c.Next()
	}
}
