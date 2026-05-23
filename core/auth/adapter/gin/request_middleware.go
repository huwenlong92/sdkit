package gin

import (
	"context"
	"net/http"

	coreauth "github.com/huwenlong92/sdkit/core/auth"
	apperrors "github.com/huwenlong92/sdkit/core/errors"
	"github.com/huwenlong92/sdkit/core/ginresponder"
	"github.com/huwenlong92/sdkit/core/session"

	"github.com/gin-gonic/gin"
)

type ginContextKey struct{}

func ContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ginContextKey{}, c))
		c.Next()
	}
}

type MiddlewareConfig struct {
	Responder ginresponder.ErrorResponder
}

type MiddlewareOption func(*MiddlewareConfig)

func WithResponder(responder ginresponder.ErrorResponder) MiddlewareOption {
	return func(cfg *MiddlewareConfig) {
		cfg.Responder = responder
	}
}

func newMiddlewareConfig(opts ...MiddlewareOption) *MiddlewareConfig {
	cfg := &MiddlewareConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}

func respondUnauthorized(cfg *MiddlewareConfig, c *gin.Context, message string) {
	ginresponder.RespondError(cfg.Responder, c, http.StatusUnauthorized, apperrors.NewCodeWithData(apperrors.CodeAuthRequired, message, nil))
}

func Required(authenticator coreauth.RequestAuthenticator, opts ...MiddlewareOption) gin.HandlerFunc {
	cfg := newMiddlewareConfig(opts...)
	return func(c *gin.Context) {
		identity, err := authenticate(c, authenticator)
		if err != nil || identity == nil || !identity.Authenticated() {
			respondUnauthorized(cfg, c, "用户未登录")
			return
		}
		SetIdentity(c, identity)
		c.Next()
	}
}

func Optional(authenticator coreauth.RequestAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, err := authenticate(c, authenticator)
		if err == nil && identity != nil && identity.Authenticated() {
			SetIdentity(c, identity)
		}
		c.Next()
	}
}

func authenticate(c *gin.Context, authenticator coreauth.RequestAuthenticator) (*coreauth.Identity, error) {
	if authenticator == nil {
		return nil, coreauth.ErrUnauthorized
	}
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ginContextKey{}, c))
	return authenticator.AuthenticateRequest(c.Request.Context(), c.Request)
}

type SessionReader struct{}

func (SessionReader) ReadSession(ctx context.Context, _ *http.Request, key string) (any, bool, error) {
	c, _ := ctx.Value(ginContextKey{}).(*gin.Context)
	if c == nil {
		return nil, false, nil
	}
	value := session.Get(c, key)
	return value, value != nil, nil
}
