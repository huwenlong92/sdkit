package checkers

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/security/audit"
	"github.com/huwenlong92/sdkit/core/security/risk"
	"github.com/huwenlong92/sdkit/core/security/state"
)

type SMSChecker struct {
	Store      state.Store
	Cooldown   time.Duration
	CodeTTL    time.Duration
	DailyLimit int64
	IPLimit    int64
}

func NewSMSChecker(store state.Store) *SMSChecker {
	return &SMSChecker{Store: store, Cooldown: time.Minute, CodeTTL: 5 * time.Minute, DailyLimit: 20, IPLimit: 50}
}

func (c *SMSChecker) Name() string { return "sms" }

func (c *SMSChecker) Check(ctx context.Context, rc *risk.Context) (*risk.CheckResult, error) {
	if rc.Scene != risk.SceneSMS {
		return nil, nil
	}
	switch rc.ExtraString("event") {
	case "sms_send":
		return c.checkSend(ctx, rc)
	case "sms_verify":
		return c.checkVerify(ctx, rc)
	default:
		return nil, nil
	}
}

func (c *SMSChecker) checkSend(ctx context.Context, rc *risk.Context) (*risk.CheckResult, error) {
	if rc.Phone == "" {
		rc.Phone = rc.ExtraString("phone")
	}
	cr := &risk.CheckResult{Passed: true}
	if rc.Phone == "" {
		return cr, nil
	}
	cooldownKey := "security:sms:cooldown:" + rc.Phone
	exists, err := c.Store.Exists(ctx, cooldownKey)
	if err != nil {
		return nil, err
	}
	if exists {
		cr.Passed = false
		cr.Actions = []risk.Action{risk.ActionDelay}
		cr.Events = []*audit.Event{event(rc, "sms_limited", levelWarn, 50, risk.ActionDelay, "cooldown active")}
		return cr, nil
	}
	daily, err := c.Store.Incr(ctx, "security:sms:daily:"+rc.Phone, 24*time.Hour)
	if err != nil {
		return nil, err
	}
	if daily > c.DailyLimit {
		cr.Passed = false
		cr.Actions = []risk.Action{risk.ActionBlock}
		cr.Blocked = true
		cr.Events = []*audit.Event{event(rc, "sms_limited", levelHigh, 100, risk.ActionBlock, "daily limit reached")}
		return cr, nil
	}
	if rc.IP != "" {
		ipCount, err := c.Store.Incr(ctx, "security:sms:ip:"+rc.IP, time.Hour)
		if err != nil {
			return nil, err
		}
		if ipCount > c.IPLimit {
			cr.Passed = false
			cr.Actions = []risk.Action{risk.ActionReview}
			cr.Events = []*audit.Event{event(rc, "sms_high_risk", levelHigh, 80, risk.ActionReview, "ip hourly limit reached")}
			return cr, nil
		}
	}
	code := rc.ExtraString("code")
	if code != "" {
		if err := c.Store.Set(ctx, "security:sms:code:"+rc.Phone, code, ttlOrDefault(c.CodeTTL, 5*time.Minute)); err != nil {
			return nil, err
		}
	}
	if err := c.Store.Set(ctx, cooldownKey, "1", ttlOrDefault(c.Cooldown, time.Minute)); err != nil {
		return nil, err
	}
	cr.Events = []*audit.Event{event(rc, "sms_send", levelInfo, 0, risk.ActionAllow, "sms sent")}
	return cr, nil
}

func (c *SMSChecker) checkVerify(ctx context.Context, rc *risk.Context) (*risk.CheckResult, error) {
	if rc.Phone == "" {
		rc.Phone = rc.ExtraString("phone")
	}
	code := rc.ExtraString("code")
	saved, ok, err := c.Store.Get(ctx, "security:sms:code:"+rc.Phone)
	if err != nil {
		return nil, err
	}
	if !ok || saved != code {
		return &risk.CheckResult{Passed: false, Actions: []risk.Action{risk.ActionVerify}, NeedVerify: true}, nil
	}
	return &risk.CheckResult{Passed: true, Actions: []risk.Action{risk.ActionAllow}}, nil
}
