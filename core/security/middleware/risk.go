package middleware

import (
	"net/http"

	apperrors "github.com/huwenlong92/sdkit/core/errors"
	"github.com/huwenlong92/sdkit/core/ginresponder"
	"github.com/huwenlong92/sdkit/core/security"
	"github.com/huwenlong92/sdkit/core/security/fingerprint"
	"github.com/huwenlong92/sdkit/core/security/risk"

	"github.com/gin-gonic/gin"
)

type RiskContextFunc func(c *gin.Context, rc *risk.Context)

func Risk(scene string, manager *risk.Manager, opts ...MiddlewareOption) gin.HandlerFunc {
	return RiskWithContext(scene, manager, nil, opts...)
}

func RiskWithContext(scene string, manager *risk.Manager, fill RiskContextFunc, opts ...MiddlewareOption) gin.HandlerFunc {
	cfg := newMiddlewareConfig(opts...)
	return func(c *gin.Context) {
		if manager == nil {
			c.Next()
			return
		}
		info := fingerprint.FromRequest(c.Request)
		rc := &risk.Context{
			Scene:    scene,
			IP:       info.IP,
			UA:       info.UA,
			DeviceID: info.DeviceID,
			Path:     info.Path,
			Method:   info.Method,
			Headers:  requestHeaders(c),
			Extra:    map[string]any{},
		}
		if fill != nil {
			fill(c, rc)
		}
		result, err := manager.Check(c.Request.Context(), rc)
		if err != nil {
			ginresponder.RespondError(cfg.Responder, c, http.StatusInternalServerError, apperrors.NewCodeWithData(security.ErrCodeSecurityInternal, security.MsgSecurityInternal, nil))
			return
		}
		if result.Blocked {
			ginresponder.RespondError(cfg.Responder, c, http.StatusOK, apperrors.NewCodeWithData(security.ErrCodeRiskBlocked, security.MsgRiskBlocked, result))
			return
		}
		if result.NeedCaptcha {
			ginresponder.RespondError(cfg.Responder, c, http.StatusOK, apperrors.NewCodeWithData(security.ErrCodeCaptchaRequired, security.MsgCaptchaRequired, result))
			return
		}
		if result.NeedVerify {
			ginresponder.RespondError(cfg.Responder, c, http.StatusOK, apperrors.NewCodeWithData(security.ErrCodeVerifyRequired, security.MsgVerifyRequired, result))
			return
		}
		c.Next()
	}
}

func requestHeaders(c *gin.Context) map[string]string {
	headers := make(map[string]string, len(c.Request.Header))
	for key, values := range c.Request.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return headers
}
