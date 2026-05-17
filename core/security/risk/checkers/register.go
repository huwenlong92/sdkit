package checkers

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/security/audit"
	"github.com/huwenlong92/sdkit/core/security/risk"
	"github.com/huwenlong92/sdkit/core/security/state"
)

type RegisterChecker struct {
	Store              state.Store
	Window             time.Duration
	BlockTTL           time.Duration
	IPCaptchaAfter     int64
	IPBlockAfter       int64
	DeviceCaptchaAfter int64
	DeviceBlockAfter   int64
}

func NewRegisterChecker(store state.Store) *RegisterChecker {
	return &RegisterChecker{Store: store, Window: time.Hour, BlockTTL: 30 * time.Minute, IPCaptchaAfter: 10, IPBlockAfter: 20, DeviceCaptchaAfter: 5, DeviceBlockAfter: 10}
}

func (c *RegisterChecker) Name() string { return "register" }

func (c *RegisterChecker) Check(ctx context.Context, rc *risk.Context) (*risk.CheckResult, error) {
	if rc.Scene != risk.SceneRegister {
		return nil, nil
	}
	cr := &risk.CheckResult{Passed: true}
	if rc.IP != "" {
		blocked, err := c.Store.Exists(ctx, "security:block:ip:"+rc.IP)
		if err != nil {
			return nil, err
		}
		if blocked {
			return &risk.CheckResult{Passed: false, Blocked: true, Actions: []risk.Action{risk.ActionBlock}, Events: []*audit.Event{event(rc, "register_ip_blocked", levelHigh, 100, risk.ActionBlock, "register ip blocked")}}, nil
		}
		n, err := c.Store.Incr(ctx, "security:register:ip:"+rc.IP, ttlOrDefault(c.Window, time.Hour))
		if err != nil {
			return nil, err
		}
		if n >= c.IPBlockAfter {
			if err := c.Store.Set(ctx, "security:block:ip:"+rc.IP, "1", ttlOrDefault(c.BlockTTL, 30*time.Minute)); err != nil {
				return nil, err
			}
			return &risk.CheckResult{Passed: false, Blocked: true, Actions: []risk.Action{risk.ActionBlock}, Events: []*audit.Event{event(rc, "register_ip_blocked", levelHigh, 100, risk.ActionBlock, "register ip threshold reached")}}, nil
		}
		if n >= c.IPCaptchaAfter {
			cr.Passed, cr.NeedCaptcha = false, true
			cr.Actions = append(cr.Actions, risk.ActionCaptcha)
			cr.Events = append(cr.Events, event(rc, "register_risk", levelWarn, 50, risk.ActionCaptcha, "register ip captcha threshold reached"))
		}
	}
	if rc.DeviceID != "" {
		blocked, err := c.Store.Exists(ctx, "security:block:device:"+rc.DeviceID)
		if err != nil {
			return nil, err
		}
		if blocked {
			return &risk.CheckResult{Passed: false, Blocked: true, Actions: []risk.Action{risk.ActionBlock}, Events: []*audit.Event{event(rc, "register_device_blocked", levelHigh, 100, risk.ActionBlock, "register device blocked")}}, nil
		}
		n, err := c.Store.Incr(ctx, "security:register:device:"+rc.DeviceID, ttlOrDefault(c.Window, time.Hour))
		if err != nil {
			return nil, err
		}
		if n >= c.DeviceBlockAfter {
			if err := c.Store.Set(ctx, "security:block:device:"+rc.DeviceID, "1", ttlOrDefault(c.BlockTTL, 30*time.Minute)); err != nil {
				return nil, err
			}
			return &risk.CheckResult{Passed: false, Blocked: true, Actions: []risk.Action{risk.ActionBlock}, Events: []*audit.Event{event(rc, "register_device_blocked", levelHigh, 100, risk.ActionBlock, "register device threshold reached")}}, nil
		}
		if n >= c.DeviceCaptchaAfter {
			cr.Passed, cr.NeedCaptcha = false, true
			cr.Actions = append(cr.Actions, risk.ActionCaptcha)
			cr.Events = append(cr.Events, event(rc, "machine_register", levelWarn, 50, risk.ActionCaptcha, "register device captcha threshold reached"))
		}
	}
	return cr, nil
}
