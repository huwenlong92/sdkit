package middleware

import (
	"io"
	"net/http"
	"time"

	"github.com/huwenlong92/sdkit/core/errors"
	"github.com/huwenlong92/sdkit/core/ginresponder"
	"github.com/huwenlong92/sdkit/core/security"
	"github.com/huwenlong92/sdkit/core/security/risk"
	"github.com/huwenlong92/sdkit/core/security/risk/checkers"
	"github.com/huwenlong92/sdkit/core/security/state"
	"github.com/huwenlong92/sdkit/pkg/security/fingerprint"

	"github.com/gin-gonic/gin"
)

func Signature(store state.Store, secret []byte, opts ...MiddlewareOption) gin.HandlerFunc {
	cfg := newMiddlewareConfig(opts...)
	checker := checkers.NewSignatureChecker(store, secret)
	checker.CheckNonce = false
	manager := risk.NewManager(nil, checker)
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			ginresponder.RespondError(cfg.Responder, c, http.StatusBadRequest, errors.NewCodeWithData(http.StatusBadRequest, "read body failed", nil))
			return
		}
		c.Request.Body = io.NopCloser(bytesReader(body))
		info := fingerprint.FromRequest(c.Request)
		result, err := manager.Check(c.Request.Context(), &risk.Context{
			Scene:    risk.SceneOpenAPI,
			IP:       info.IP,
			UA:       info.UA,
			DeviceID: info.DeviceID,
			Path:     info.Path,
			Method:   info.Method,
			Body:     body,
			Headers:  requestHeaders(c),
		})
		if err != nil {
			ginresponder.RespondError(cfg.Responder, c, http.StatusInternalServerError, errors.NewCodeWithData(security.ErrCodeSecurityInternal, security.MsgSecurityInternal, nil))
			return
		}
		if !result.Passed {
			ginresponder.RespondError(cfg.Responder, c, http.StatusOK, errors.NewCodeWithData(security.ErrCodeInvalidSign, security.MsgInvalidSign, result))
			return
		}
		c.Next()
	}
}

func Replay(store state.Store, opts ...MiddlewareOption) gin.HandlerFunc {
	cfg := newMiddlewareConfig(opts...)
	return func(c *gin.Context) {
		nonce := c.GetHeader("U-Nonce")
		if nonce == "" {
			ginresponder.RespondError(cfg.Responder, c, http.StatusOK, errors.NewCodeWithData(security.ErrCodeNonceRequired, security.MsgNonceRequired, nil))
			return
		}
		ok, err := store.SetNX(c.Request.Context(), "security:nonce:"+nonce, "1", 5*time.Minute)
		if err != nil {
			ginresponder.RespondError(cfg.Responder, c, http.StatusInternalServerError, errors.NewCodeWithData(security.ErrCodeSecurityInternal, security.MsgSecurityInternal, nil))
			return
		}
		if !ok {
			ginresponder.RespondError(cfg.Responder, c, http.StatusOK, errors.NewCodeWithData(security.ErrCodeNonceReplay, security.MsgNonceReplay, nil))
			return
		}
		c.Next()
	}
}
