// Package keyer 提供限流 key 提取函数
package keyer

import "github.com/gin-gonic/gin"

// IP 客户端 IP
func IP(c *gin.Context) string {
	return c.ClientIP()
}
