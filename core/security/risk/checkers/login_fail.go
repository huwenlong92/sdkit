package checkers

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/security/audit"
	"github.com/huwenlong92/sdkit/core/security/risk"
	"github.com/huwenlong92/sdkit/core/security/state"
)

type LoginFailChecker struct {
	Store        state.Store
	Window       time.Duration
	BlockTTL     time.Duration
	CaptchaAfter int64
	UIDBlockAt   int64
	IPBlockAt    int64
}

func NewLoginFailChecker(store state.Store) *LoginFailChecker {
	return &LoginFailChecker{Store: store, Window: 15 * time.Minute, BlockTTL: 30 * time.Minute, CaptchaAfter: 5, UIDBlockAt: 10, IPBlockAt: 20}
}

func (c *LoginFailChecker) Name() string { return "login_fail" }

func (c *LoginFailChecker) Check(ctx context.Context, rc *risk.Context) (*risk.CheckResult, error) {
	if rc.Scene != risk.SceneLogin || rc.ExtraString("event") != "login_failed" {
		return nil, nil
	}
	blockTTL := ttlOrDefault(c.BlockTTL, 30*time.Minute)
	if rc.UID > 0 {
		blocked, err := c.Store.Exists(ctx, "security:block:uid:"+uidString(rc.UID))
		if err != nil {
			return nil, err
		}
		if blocked {
			return &risk.CheckResult{Passed: false, Blocked: true, Actions: []risk.Action{risk.ActionBlock}, Events: []*audit.Event{event(rc, "login_uid_blocked", levelHigh, 100, risk.ActionBlock, "uid blocked")}}, nil
		}
	}
	if rc.IP != "" {
		blocked, err := c.Store.Exists(ctx, "security:block:ip:"+rc.IP)
		if err != nil {
			return nil, err
		}
		if blocked {
			return &risk.CheckResult{Passed: false, Blocked: true, Actions: []risk.Action{risk.ActionBlock}, Events: []*audit.Event{event(rc, "login_ip_blocked", levelHigh, 100, risk.ActionBlock, "ip blocked")}}, nil
		}
	}
	cr := &risk.CheckResult{Passed: true, Events: []*audit.Event{event(rc, "login_failed", levelInfo, 10, risk.ActionAllow, "login failed")}}
	if rc.UID > 0 {
		n, err := c.Store.Incr(ctx, "security:login_fail:uid:"+uidString(rc.UID), ttlOrDefault(c.Window, 15*time.Minute))
		if err != nil {
			return nil, err
		}
		if n >= c.UIDBlockAt {
			if err := c.Store.Set(ctx, "security:block:uid:"+uidString(rc.UID), "1", blockTTL); err != nil {
				return nil, err
			}
			cr.Passed, cr.Blocked = false, true
			cr.Actions = append(cr.Actions, risk.ActionBlock)
			cr.Events = append(cr.Events, event(rc, "login_uid_blocked", levelHigh, 100, risk.ActionBlock, "uid failure threshold reached"))
			return cr, nil
		}
		if n >= c.CaptchaAfter {
			cr.Passed, cr.NeedCaptcha = false, true
			cr.Actions = append(cr.Actions, risk.ActionCaptcha)
			cr.Events = append(cr.Events, event(rc, "login_captcha_required", levelWarn, 50, risk.ActionCaptcha, "uid captcha threshold reached"))
		}
	}
	if rc.IP != "" {
		n, err := c.Store.Incr(ctx, "security:login_fail:ip:"+rc.IP, ttlOrDefault(c.Window, 15*time.Minute))
		if err != nil {
			return nil, err
		}
		if n >= c.IPBlockAt {
			if err := c.Store.Set(ctx, "security:block:ip:"+rc.IP, "1", blockTTL); err != nil {
				return nil, err
			}
			cr.Passed, cr.Blocked = false, true
			cr.NeedCaptcha = false
			cr.Actions = append(cr.Actions, risk.ActionBlock)
			cr.Events = append(cr.Events, event(rc, "login_ip_blocked", levelHigh, 100, risk.ActionBlock, "ip failure threshold reached"))
		}
	}
	return cr, nil
}
