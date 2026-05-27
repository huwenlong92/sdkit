package keyer

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	ContextSubjectKey  = "ratelimit.subject_key"
	ContextSubjectType = "ratelimit.subject_type"
	ContextSubjectID   = "ratelimit.subject_id"
)

func SetSubject(c *gin.Context, subjectType string, subjectID any) {
	c.Set(ContextSubjectType, subjectType)
	c.Set(ContextSubjectID, subjectID)
}

func SetSubjectKey(c *gin.Context, key string) {
	c.Set(ContextSubjectKey, key)
}

// User 按认证主体提取 key，格式 "subject:<type>:<id>"，无主体时返回空字符串
func User(c *gin.Context) string {
	if key := c.GetString(ContextSubjectKey); key != "" {
		return key
	}
	subjectType := c.GetString(ContextSubjectType)
	subjectID := subjectIDString(c)
	if subjectType == "" || subjectID == "" || subjectID == "0" {
		return ""
	}
	return fmt.Sprintf("subject:%s:%s", subjectType, subjectID)
}

// UserRoute 按「用户 + 方法 + 路径」提取 key
func UserRoute(c *gin.Context) string {
	key := User(c)
	if key == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s:%s", key, c.Request.Method, c.Request.URL.Path)
}

func subjectIDString(c *gin.Context) string {
	value, ok := c.Get(ContextSubjectID)
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	default:
		return fmt.Sprint(v)
	}
}
