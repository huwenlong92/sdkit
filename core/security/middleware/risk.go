package middleware

import (
	apperrors "github.com/huwenlong92/sdkit/core/errors"
	"github.com/huwenlong92/sdkit/core/response"
	"github.com/huwenlong92/sdkit/core/security"
	"github.com/huwenlong92/sdkit/core/security/fingerprint"
	"github.com/huwenlong92/sdkit/core/security/risk"

	"github.com/gin-gonic/gin"
)

type RiskContextFunc func(c *gin.Context, rc *risk.Context)

func Risk(scene string, manager *risk.Manager) gin.HandlerFunc {
	return RiskWithContext(scene, manager, nil)
}

func RiskWithContext(scene string, manager *risk.Manager, fill RiskContextFunc) gin.HandlerFunc {
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
			response.Error(c, apperrors.NewCodeWithData(security.ErrCodeSecurityInternal, security.MsgSecurityInternal, nil))
			c.Abort()
			return
		}
		if result.Blocked {
			response.Error(c, apperrors.NewCodeWithData(security.ErrCodeRiskBlocked, security.MsgRiskBlocked, result))
			c.Abort()
			return
		}
		if result.NeedCaptcha {
			response.Error(c, apperrors.NewCodeWithData(security.ErrCodeCaptchaRequired, security.MsgCaptchaRequired, result))
			c.Abort()
			return
		}
		if result.NeedVerify {
			response.Error(c, apperrors.NewCodeWithData(security.ErrCodeVerifyRequired, security.MsgVerifyRequired, result))
			c.Abort()
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
