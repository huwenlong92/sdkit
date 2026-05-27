package gin

import (
	"context"
	"net/http"

	"github.com/huwenlong92/sdkit/core/auth"
	"github.com/huwenlong92/sdkit/core/errors"
	ginresponder "github.com/huwenlong92/sdkit/core/gin/responder"
	"github.com/huwenlong92/sdkit/core/gin/session"

	"github.com/gin-gonic/gin"
)

type ginContextKey struct{}

func WithContext(ctx context.Context, c *gin.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, ginContextKey{}, c)
}

func ContextFrom(ctx context.Context) (*gin.Context, bool) {
	if ctx == nil {
		return nil, false
	}
	c, ok := ctx.Value(ginContextKey{}).(*gin.Context)
	return c, ok && c != nil
}

func ContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(WithContext(c.Request.Context(), c))
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
	ginresponder.RespondError(cfg.Responder, c, http.StatusUnauthorized, errors.NewCodeWithData(errors.CodeAuthRequired, message, nil))
}

func Required(authenticator auth.RequestAuthenticator, opts ...MiddlewareOption) gin.HandlerFunc {
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

func Optional(authenticator auth.RequestAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, err := authenticate(c, authenticator)
		if err == nil && identity != nil && identity.Authenticated() {
			SetIdentity(c, identity)
		}
		c.Next()
	}
}

func authenticate(c *gin.Context, authenticator auth.RequestAuthenticator) (*auth.Identity, error) {
	if authenticator == nil {
		return nil, auth.ErrUnauthorized
	}
	c.Request = c.Request.WithContext(WithContext(c.Request.Context(), c))
	return authenticator.AuthenticateRequest(c.Request.Context(), c.Request)
}

type SessionReader struct{}

func (SessionReader) ReadSession(ctx context.Context, _ *http.Request, key string) (any, bool, error) {
	c, ok := ContextFrom(ctx)
	if !ok {
		return nil, false, nil
	}
	value := session.Get(c, key)
	return value, value != nil, nil
}
