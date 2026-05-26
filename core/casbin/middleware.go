package casbin

import (
	"net/http"

	"github.com/huwenlong92/sdkit/core/errors"
	"github.com/huwenlong92/sdkit/core/ginresponder"

	"github.com/gin-gonic/gin"
)

type RoleResolver func(c *gin.Context) (string, bool)
type ObjectResolver func(c *gin.Context) string

type MiddlewareConfig struct {
	Manager        *Manager
	RoleResolver   RoleResolver
	ObjectResolver ObjectResolver
	Responder      ginresponder.ErrorResponder
}

type MiddlewareOption func(*MiddlewareConfig)

func WithManager(manager *Manager) MiddlewareOption {
	return func(cfg *MiddlewareConfig) {
		cfg.Manager = manager
	}
}

func WithRoleResolver(resolver RoleResolver) MiddlewareOption {
	return func(cfg *MiddlewareConfig) {
		cfg.RoleResolver = resolver
	}
}

func WithObjectResolver(resolver ObjectResolver) MiddlewareOption {
	return func(cfg *MiddlewareConfig) {
		cfg.ObjectResolver = resolver
	}
}

func WithResponder(responder ginresponder.ErrorResponder) MiddlewareOption {
	return func(cfg *MiddlewareConfig) {
		cfg.Responder = responder
	}
}

func Middleware(opts ...MiddlewareOption) gin.HandlerFunc {
	cfg := &MiddlewareConfig{
		Manager:        Default,
		ObjectResolver: defaultObjectResolver,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	return func(c *gin.Context) {
		if cfg.Manager == nil || cfg.Manager.Enforcer() == nil || cfg.RoleResolver == nil {
			c.Next()
			return
		}

		role, ok := cfg.RoleResolver(c)
		if !ok || role == "" {
			c.Next()
			return
		}

		obj := cfg.ObjectResolver(c)
		act := c.Request.Method
		allowed, err := cfg.Manager.Enforce(role, obj, act)
		if err != nil || !allowed {
			ginresponder.RespondError(cfg.Responder, c, http.StatusForbidden, errors.NewCodeWithData(errors.CodeForbidden, "无权访问该接口", nil))
			return
		}
		c.Next()
	}
}

func defaultObjectResolver(c *gin.Context) string {
	return c.Request.URL.Path
}
