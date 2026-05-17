package middleware

import (
	"io"
	"net/http"
	"time"

	apperrors "github.com/huwenlong92/sdkit/core/errors"
	"github.com/huwenlong92/sdkit/core/response"
	"github.com/huwenlong92/sdkit/core/security"
	"github.com/huwenlong92/sdkit/core/security/fingerprint"
	"github.com/huwenlong92/sdkit/core/security/risk"
	"github.com/huwenlong92/sdkit/core/security/risk/checkers"
	"github.com/huwenlong92/sdkit/core/security/state"

	"github.com/gin-gonic/gin"
)

func Signature(store state.Store, secret []byte) gin.HandlerFunc {
	checker := checkers.NewSignatureChecker(store, secret)
	checker.CheckNonce = false
	manager := risk.NewManager(nil, checker)
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			response.Error(c, apperrors.NewCodeWithData(http.StatusBadRequest, "read body failed", nil))
			c.Abort()
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
			response.Error(c, apperrors.NewCodeWithData(security.ErrCodeSecurityInternal, security.MsgSecurityInternal, nil))
			c.Abort()
			return
		}
		if !result.Passed {
			response.Error(c, apperrors.NewCodeWithData(security.ErrCodeInvalidSign, security.MsgInvalidSign, result))
			c.Abort()
			return
		}
		c.Next()
	}
}

func Replay(store state.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		nonce := c.GetHeader("U-Nonce")
		if nonce == "" {
			response.Error(c, apperrors.NewCodeWithData(security.ErrCodeNonceRequired, security.MsgNonceRequired, nil))
			c.Abort()
			return
		}
		ok, err := store.SetNX(c.Request.Context(), "security:nonce:"+nonce, "1", 5*time.Minute)
		if err != nil {
			response.Error(c, apperrors.NewCodeWithData(security.ErrCodeSecurityInternal, security.MsgSecurityInternal, nil))
			c.Abort()
			return
		}
		if !ok {
			response.Error(c, apperrors.NewCodeWithData(security.ErrCodeNonceReplay, security.MsgNonceReplay, nil))
			c.Abort()
			return
		}
		c.Next()
	}
}
