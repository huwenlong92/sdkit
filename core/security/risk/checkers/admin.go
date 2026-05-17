package checkers

import (
	"context"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/core/security/audit"
	"github.com/huwenlong92/sdkit/core/security/risk"
	"github.com/huwenlong92/sdkit/core/security/state"
)

type AdminChecker struct {
	Store        state.Store
	Window       time.Duration
	BlockTTL     time.Duration
	CaptchaAfter int64
	BlockAfter   int64
	ScanPaths    []string
}

func NewAdminChecker(store state.Store) *AdminChecker {
	return &AdminChecker{
		Store:        store,
		Window:       15 * time.Minute,
		BlockTTL:     30 * time.Minute,
		CaptchaAfter: 5,
		BlockAfter:   10,
		ScanPaths:    []string{"/wp-admin", "/phpmyadmin", "/.env", "/.git/config", "/adminer.php"},
	}
}

func (c *AdminChecker) Name() string { return "admin" }

func (c *AdminChecker) Check(ctx context.Context, rc *risk.Context) (*risk.CheckResult, error) {
	if rc.Scene != risk.SceneAdmin {
		return nil, nil
	}
	for _, path := range c.ScanPaths {
		if strings.EqualFold(rc.Path, path) {
			return &risk.CheckResult{Passed: false, Score: 80, Actions: []risk.Action{risk.ActionReview}, Events: []*audit.Event{event(rc, "admin_scan_detected", levelHigh, 80, risk.ActionReview, "scan path detected")}}, nil
		}
	}
	if rc.ExtraString("event") != "admin_login_failed" {
		return nil, nil
	}
	if rc.IP == "" {
		return &risk.CheckResult{Passed: true}, nil
	}
	blockKey := "security:block:ip:" + rc.IP
	blocked, err := c.Store.Exists(ctx, blockKey)
	if err != nil {
		return nil, err
	}
	if blocked {
		return &risk.CheckResult{Passed: false, Blocked: true, Actions: []risk.Action{risk.ActionBlock}, Events: []*audit.Event{event(rc, "admin_ip_blocked", levelHigh, 100, risk.ActionBlock, "admin ip blocked")}}, nil
	}
	n, err := c.Store.Incr(ctx, "security:admin_fail:ip:"+rc.IP, ttlOrDefault(c.Window, 15*time.Minute))
	if err != nil {
		return nil, err
	}
	cr := &risk.CheckResult{Passed: true, Events: []*audit.Event{event(rc, "admin_login_failed", levelInfo, 10, risk.ActionAllow, "admin login failed")}}
	if n >= c.BlockAfter {
		if err := c.Store.Set(ctx, blockKey, "1", ttlOrDefault(c.BlockTTL, 30*time.Minute)); err != nil {
			return nil, err
		}
		cr.Passed, cr.Blocked = false, true
		cr.Actions = []risk.Action{risk.ActionBlock}
		cr.Events = append(cr.Events, event(rc, "admin_ip_blocked", levelHigh, 100, risk.ActionBlock, "admin failure threshold reached"))
		return cr, nil
	}
	if n >= c.CaptchaAfter {
		cr.Passed, cr.NeedCaptcha = false, true
		cr.Actions = []risk.Action{risk.ActionCaptcha}
		cr.Events = append(cr.Events, event(rc, "admin_captcha_required", levelWarn, 50, risk.ActionCaptcha, "admin captcha threshold reached"))
	}
	return cr, nil
}
