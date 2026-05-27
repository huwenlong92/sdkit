package ginrequestid

import (
	"github.com/gin-gonic/gin"
	"github.com/huwenlong92/sdkit/core/requestid"
)

func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestid.Header)
		if id == "" {
			id = requestid.New()
		}
		c.Set(requestid.Key, id)
		c.Header(requestid.Header, id)
		ctx := requestid.WithRequestID(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func Get(c *gin.Context) string {
	id, _ := c.Get(requestid.Key)
	if id == nil {
		return ""
	}
	return id.(string)
}
