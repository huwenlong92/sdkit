package gin

import (
	"net/http"
	"strings"

	coreauth "github.com/huwenlong92/sdkit/core/auth"
	"github.com/huwenlong92/sdkit/core/response"

	"github.com/gin-gonic/gin"
)

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
