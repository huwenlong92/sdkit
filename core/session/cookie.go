package session

import (
	"time"

	"github.com/huwenlong92/sdkit/pkg/sessionx"

	"github.com/gin-gonic/gin"
)

const (
	CookieName        = "sid"
	RenewThreshold    = 5 * time.Minute
	HeaderExpireKey   = "X-Session-Expires-At"
	ContextSessionKey = "session.current"
)

// SetCookie 设置会话 cookie
func SetCookie(c *gin.Context, sid string, ttl time.Duration) {
	secure := c.Request.TLS != nil
	cookie := sessionx.NewCookie(sid, sessionx.CookieOptions{
		Name:     CookieName,
		TTL:      ttl,
		Secure:   secure,
		HTTPOnly: true,
	})
	c.SetSameSite(cookie.SameSite)
	c.SetCookie(cookie.Name, cookie.Value, cookie.MaxAge, cookie.Path, cookie.Domain, cookie.Secure, cookie.HttpOnly)
	c.Header(HeaderExpireKey, time.Now().Add(ttl).Format(time.RFC3339))
}

// ClearCookie 清除会话 cookie
func ClearCookie(c *gin.Context) {
	cookie := sessionx.NewClearCookie(sessionx.CookieOptions{Name: CookieName, HTTPOnly: true})
	c.SetCookie(cookie.Name, cookie.Value, cookie.MaxAge, cookie.Path, cookie.Domain, cookie.Secure, cookie.HttpOnly)
}
