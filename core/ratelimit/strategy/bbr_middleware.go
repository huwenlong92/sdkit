package strategy

import (
	"net/http"

	"github.com/huwenlong92/sdkit/core/response"

	"github.com/gin-gonic/gin"
)

// BBRMiddleware 返回 BBR Gin 中间件
func BBRMiddleware(l *BBR) gin.HandlerFunc {
	return func(c *gin.Context) {
		done, err := l.Allow()
		if err != nil {
			response.AbortJSON(c, http.StatusTooManyRequests, gin.H{
				"err_code": http.StatusTooManyRequests,
				"msg":      "服务繁忙，请稍后再试",
			})
			return
		}
		c.Next()
		done()
	}
}
