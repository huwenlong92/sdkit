package session

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/response"

	"github.com/gin-gonic/gin"
)

func WithStore(store Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if store != nil {
			c.Request = c.Request.WithContext(ContextWithStore(c.Request.Context(), store))
		}
		c.Next()
	}
}

// Require 强制登录，未登录返回 401
func Require(store Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		sid, err := c.Cookie(CookieName)
		if err != nil {
			response.AbortJSON(c, 401, gin.H{"err_code": 4001, "msg": "用户未登录"})
			return
		}

		s, ok := store.Get(c.Request.Context(), sid)
		if !ok {
			ClearCookie(c)
			response.AbortJSON(c, 401, gin.H{"err_code": 4001, "msg": "会话已过期"})
			return
		}

		// 续期
		if time.Until(s.ExpiresAt) < RenewThreshold {
			store.Set(context.Background(), s, SessionTTL)
			SetCookie(c, sid, SessionTTL)
		}

		c.Set(ContextSessionKey, s)
		c.Next()
	}
}

// Optional 检测登录状态但不强制
func Optional(store Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		sid, err := c.Cookie(CookieName)
		if err != nil {
			c.Next()
			return
		}

		s, ok := store.Get(c.Request.Context(), sid)
		if !ok {
			c.Next()
			return
		}

		c.Set(ContextSessionKey, s)
		c.Next()
	}
}

func GetSession(c *gin.Context) *Session {
	v, ok := c.Get(ContextSessionKey)
	if !ok {
		return nil
	}
	sess, _ := v.(*Session)
	return sess
}
