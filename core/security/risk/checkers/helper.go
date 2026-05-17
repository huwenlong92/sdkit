package checkers

import (
	"strconv"
	"time"

	"github.com/huwenlong92/sdkit/core/security/audit"
	"github.com/huwenlong92/sdkit/core/security/risk"
)

const (
	levelInfo = "info"
	levelWarn = "warn"
	levelHigh = "high"
)

func event(rc *risk.Context, name, level string, score int, action risk.Action, reason string) *audit.Event {
	return &audit.Event{
		Scene:    rc.Scene,
		Event:    name,
		Level:    level,
		UID:      rc.UID,
		IP:       rc.IP,
		DeviceID: rc.DeviceID,
		Score:    score,
		Action:   string(action),
		Reason:   map[string]any{"reason": reason},
	}
}

func uidString(uid int64) string {
	return strconv.FormatInt(uid, 10)
}

func result(score int, action risk.Action, reason risk.Reason) *risk.CheckResult {
	cr := &risk.CheckResult{Passed: true, Score: score}
	if action != "" {
		cr.Actions = append(cr.Actions, action)
	}
	if reason.Code != "" {
		cr.Reasons = append(cr.Reasons, reason)
	}
	return cr
}

func ttlOrDefault(ttl time.Duration, defaultValue time.Duration) time.Duration {
	if ttl > 0 {
		return ttl
	}
	return defaultValue
}
