package keyer

import (
	"fmt"

	authgin "github.com/huwenlong92/sdkit/core/auth/adapter/gin"

	"github.com/gin-gonic/gin"
)

// User 按认证主体提取 key，格式 "subject:<type>:<id>"，无主体时返回空字符串
func User(c *gin.Context) string {
	identity := authgin.GetIdentity(c)
	if identity == nil || identity.SubjectID == 0 || identity.SubjectType == "" {
		return ""
	}
	return fmt.Sprintf("subject:%s:%d", identity.SubjectType, identity.SubjectID)
}

// UserRoute 按「用户 + 方法 + 路径」提取 key
func UserRoute(c *gin.Context) string {
	identity := authgin.GetIdentity(c)
	if identity == nil || identity.SubjectID == 0 || identity.SubjectType == "" {
		return ""
	}
	return fmt.Sprintf("subject:%s:%d:%s:%s", identity.SubjectType, identity.SubjectID, c.Request.Method, c.Request.URL.Path)
}
