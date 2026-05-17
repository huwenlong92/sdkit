package gin

import (
	"context"
	"net/http"
	"strings"
	"time"

	coreauth "github.com/huwenlong92/sdkit/core/auth"
	"github.com/huwenlong92/sdkit/core/response"
	"github.com/huwenlong92/sdkit/core/session"

	"github.com/gin-gonic/gin"
)

type sessionRefresher interface {
	Refresh(ctx context.Context, sessionID string) bool
	TTL() time.Duration
}

func AuthMiddleware(manager *coreauth.Auth) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			response.AbortJSON(c, http.StatusUnauthorized, gin.H{"err_code": 4001, "msg": "用户未登录"})
			return
		}
		credential, err := credentialFromRequest(c, manager.Mode())
		if err != nil {
			response.AbortJSON(c, http.StatusUnauthorized, gin.H{"err_code": 4001, "msg": "用户未登录"})
			return
		}
		identity, err := manager.Authenticate(c.Request.Context(), credential)
		if err != nil {
			response.AbortJSON(c, http.StatusUnauthorized, gin.H{"err_code": 4001, "msg": "令牌无效或已过期"})
			return
		}
		SetIdentity(c, identity)
		refreshSessionCookie(c, manager, credential)
		c.Next()
	}
}

func OptionalAuthMiddleware(manager *coreauth.Auth) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.Next()
			return
		}
		credential, err := credentialFromRequest(c, manager.Mode())
		if err == nil {
			if identity, authErr := manager.Authenticate(c.Request.Context(), credential); authErr == nil {
				SetIdentity(c, identity)
				refreshSessionCookie(c, manager, credential)
			}
		}
		c.Next()
	}
}

func LoginByCredentials(c *gin.Context, manager *coreauth.Auth, credentials coreauth.Credentials) (*coreauth.LoginResult, error) {
	if manager == nil {
		return nil, coreauth.ErrUnauthorized
	}
	result, err := manager.LoginByCredentials(c.Request.Context(), credentials)
	if err != nil {
		return nil, err
	}
	if result.Mode == coreauth.ModeSession && result.SessionID != "" {
		session.SetCookie(c, result.SessionID, sessionTTL(manager))
	}
	return result, nil
}

func Logout(c *gin.Context, manager *coreauth.Auth) error {
	if manager == nil {
		return nil
	}
	credential, _ := credentialFromRequest(c, manager.Mode())
	if err := manager.Logout(c.Request.Context(), credential); err != nil {
		return err
	}
	if manager.Mode() == coreauth.ModeSession {
		session.ClearCookie(c)
	}
	return nil
}

func credentialFromRequest(c *gin.Context, mode coreauth.Mode) (string, error) {
	switch mode {
	case coreauth.ModeJWT:
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			return "", coreauth.ErrUnauthorized
		}
		return token, nil
	case coreauth.ModeSession:
		sid, err := c.Cookie(session.CookieName)
		if err != nil || sid == "" {
			return "", coreauth.ErrUnauthorized
		}
		return sid, nil
	default:
		return "", coreauth.ErrUnauthorized
	}
}

func bearerToken(header string) string {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ""
	}
	return parts[1]
}

func refreshSessionCookie(c *gin.Context, manager *coreauth.Auth, credential string) {
	refresher, ok := guard(manager).(sessionRefresher)
	if !ok || manager.Mode() != coreauth.ModeSession {
		return
	}
	if refresher.Refresh(c.Request.Context(), credential) {
		session.SetCookie(c, credential, refresher.TTL())
	}
}

func sessionTTL(manager *coreauth.Auth) time.Duration {
	refresher, ok := guard(manager).(sessionRefresher)
	if !ok {
		return session.SessionTTL
	}
	return refresher.TTL()
}

func guard(manager *coreauth.Auth) coreauth.Guard {
	if manager == nil {
		return nil
	}
	return manager.Guard()
}
