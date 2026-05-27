package middleware

import "github.com/gin-gonic/gin"

type SecureHeaderOption struct {
	CSP string
}

func SecureHeaders(opt SecureHeaderOption) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		if opt.CSP != "" {
			c.Header("Content-Security-Policy", opt.CSP)
		}
		c.Next()
	}
}
