package risk

import (
	"net/http"
	"strconv"

	coreauth "github.com/huwenlong92/sdkit/core/auth"
	apperrors "github.com/huwenlong92/sdkit/core/errors"
	ginresponder "github.com/huwenlong92/sdkit/core/gin/responder"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/security"
	corerisk "github.com/huwenlong92/sdkit/core/security/risk"
	"github.com/huwenlong92/sdkit/core/tracking"

	"github.com/gin-gonic/gin"
)

type Option func(*options)

type options struct {
	responder ginresponder.ErrorResponder
	extra     func(*gin.Context) map[string]any
	onError   func(*gin.Context, error)
}

func WithResponder(responder ginresponder.ErrorResponder) Option {
	return func(opts *options) {
		opts.responder = responder
	}
}

func WithExtra(fn func(*gin.Context) map[string]any) Option {
	return func(opts *options) {
		opts.extra = fn
	}
}

func WithErrorHandler(fn func(*gin.Context, error)) Option {
	return func(opts *options) {
		opts.onError = fn
	}
}

func EventFromGin(c *gin.Context, base corerisk.Event) corerisk.Event {
	if base.Extra == nil {
		base.Extra = map[string]any{}
	}
	base.Extra["path"] = c.FullPath()
	base.Extra["method"] = c.Request.Method
	base.IP = firstNonEmpty(base.IP, c.ClientIP())
	base.DeviceID = firstNonEmpty(base.DeviceID, c.GetHeader("X-Device-Id"))
	base.TrackID = firstNonEmpty(base.TrackID, tracking.TrackID(c.Request.Context()))
	base.RequestID = firstNonEmpty(base.RequestID, requestid.FromContext(c.Request.Context()))
	if identity := coreauth.CurrentIdentity(c); identity != nil {
		if base.SubjectType == "" {
			base.SubjectType = identity.SubjectType
		}
		if base.SubjectID == "" {
			base.SubjectID = strconv.FormatInt(identity.SubjectID, 10)
		}
		if base.UID == "" && identity.SubjectType == "user" {
			base.UID = base.SubjectID
		}
	}
	return base
}

func Evaluate(c *gin.Context, engine *corerisk.Engine, base corerisk.Event) (*corerisk.Decision, error) {
	return engine.Evaluate(c.Request.Context(), EventFromGin(c, base))
}

func Check(c *gin.Context, engine *corerisk.Engine, base corerisk.Event, opts ...Option) bool {
	cfg := newOptions(opts...)
	decision, err := Evaluate(c, engine, base)
	if err != nil {
		if cfg.onError != nil {
			cfg.onError(c, err)
		}
		internalError(c, cfg)
		return false
	}
	if !decision.Passed {
		Abort(c, decision, WithResponder(cfg.responder))
		return false
	}
	c.Set("risk_decision", decision)
	return true
}

func Record(c *gin.Context, engine *corerisk.Engine, base corerisk.Event, opts ...Option) bool {
	cfg := newOptions(opts...)
	decision, err := Evaluate(c, engine, base)
	if err != nil {
		if cfg.onError != nil {
			cfg.onError(c, err)
		}
		return false
	}
	c.Set("risk_decision", decision)
	return true
}

func Abort(c *gin.Context, decision *corerisk.Decision, opts ...Option) {
	cfg := newOptions(opts...)
	status := http.StatusForbidden
	if decision != nil && decision.Action == corerisk.ActionLimit {
		status = http.StatusTooManyRequests
	}
	code := security.ErrCodeRiskBlocked
	message := "请求触发风控规则"
	if decision != nil {
		if decision.HTTPStatus > 0 {
			status = decision.HTTPStatus
		}
		if decision.ResponseCode > 0 {
			code = decision.ResponseCode
		}
		if decision.ResponseMessage != "" {
			message = decision.ResponseMessage
		}
	}
	ginresponder.RespondError(cfg.responder, c, status, apperrors.NewWithData(
		code,
		security.MsgRiskBlocked,
		message,
		decision,
	))
}

func Middleware(engine *corerisk.Engine, base corerisk.Event, opts ...Option) gin.HandlerFunc {
	cfg := newOptions(opts...)
	return func(c *gin.Context) {
		event := base
		if cfg.extra != nil {
			if event.Extra == nil {
				event.Extra = map[string]any{}
			}
			for key, value := range cfg.extra(c) {
				event.Extra[key] = value
			}
		}
		decision, err := Evaluate(c, engine, event)
		if err != nil {
			if cfg.onError != nil {
				cfg.onError(c, err)
			}
			internalError(c, cfg)
			return
		}
		if !decision.Passed {
			Abort(c, decision, WithResponder(cfg.responder))
			return
		}
		c.Set("risk_decision", decision)
		c.Next()
	}
}

func internalError(c *gin.Context, cfg options) {
	ginresponder.RespondError(cfg.responder, c, http.StatusInternalServerError, apperrors.NewCodeWithData(
		security.ErrCodeSecurityInternal,
		security.MsgSecurityInternal,
		nil,
	))
}

func newOptions(opts ...Option) options {
	cfg := options{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
