package checkers

import (
	"context"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/core/security/audit"
	"github.com/huwenlong92/sdkit/core/security/risk"
	"github.com/huwenlong92/sdkit/core/security/state"
)

type AnomalyLoginChecker struct {
	Store     state.Store
	RecordTTL time.Duration
	VerifyAt  int
}

func NewAnomalyLoginChecker(store state.Store) *AnomalyLoginChecker {
	return &AnomalyLoginChecker{Store: store, RecordTTL: 24 * time.Hour * 365, VerifyAt: 40}
}

func (c *AnomalyLoginChecker) Name() string { return "anomaly_login" }

func (c *AnomalyLoginChecker) Check(ctx context.Context, rc *risk.Context) (*risk.CheckResult, error) {
	if rc.Scene != risk.SceneLoginRisk {
		return nil, nil
	}
	if rc.UID <= 0 {
		return &risk.CheckResult{Passed: true}, nil
	}
	score := 0
	reasons := make([]risk.Reason, 0, 3)
	deviceKey := "security:device_seen:" + uidString(rc.UID) + ":" + rc.DeviceID
	if rc.DeviceID != "" {
		seen, err := c.Store.Exists(ctx, deviceKey)
		if err != nil {
			return nil, err
		}
		if !seen {
			score += 20
			reasons = append(reasons, risk.Reason{Code: "new_device", Message: "new device"})
		}
	}
	lastKey := "security:last_login:uid:" + uidString(rc.UID)
	last, ok, err := c.Store.Get(ctx, lastKey)
	if err != nil {
		return nil, err
	}
	if ok {
		parts := strings.SplitN(last, "|", 2)
		if len(parts) == 2 {
			if rc.Region != "" && parts[0] != "" && parts[0] != rc.Region {
				score += 30
				reasons = append(reasons, risk.Reason{Code: "region_changed", Message: "region changed"})
			}
			if rc.UA != "" && parts[1] != "" && parts[1] != rc.UA {
				score += 10
				reasons = append(reasons, risk.Reason{Code: "ua_changed", Message: "ua changed"})
			}
		}
	}
	if rc.DeviceID != "" {
		if err := c.Store.Set(ctx, deviceKey, "1", ttlOrDefault(c.RecordTTL, 24*time.Hour*365)); err != nil {
			return nil, err
		}
	}
	if err := c.Store.Set(ctx, lastKey, rc.Region+"|"+rc.UA, ttlOrDefault(c.RecordTTL, 24*time.Hour*365)); err != nil {
		return nil, err
	}
	cr := &risk.CheckResult{Passed: true, Score: score, Reasons: reasons}
	if score >= c.VerifyAt {
		cr.Passed, cr.NeedVerify = false, true
		cr.Actions = []risk.Action{risk.ActionVerify}
		cr.Events = []*audit.Event{event(rc, "abnormal_login", levelWarn, score, risk.ActionVerify, "login risk threshold reached")}
	}
	return cr, nil
}
