package checkers

import (
	"context"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/core/security/audit"
	"github.com/huwenlong92/sdkit/core/security/risk"
	"github.com/huwenlong92/sdkit/core/security/state"
)

type CommentChecker struct {
	Store          state.Store
	Window         time.Duration
	UIDLimit       int64
	IPLimit        int64
	DeviceLimit    int64
	SensitiveWords []string
}

func NewCommentChecker(store state.Store, sensitiveWords ...string) *CommentChecker {
	return &CommentChecker{Store: store, Window: time.Minute, UIDLimit: 10, IPLimit: 30, DeviceLimit: 20, SensitiveWords: sensitiveWords}
}

func (c *CommentChecker) Name() string { return "comment" }

func (c *CommentChecker) Check(ctx context.Context, rc *risk.Context) (*risk.CheckResult, error) {
	if rc.Scene != risk.SceneComment {
		return nil, nil
	}
	cr := &risk.CheckResult{Passed: true}
	content := strings.ToLower(rc.ExtraString("content"))
	for _, word := range c.SensitiveWords {
		if word != "" && strings.Contains(content, strings.ToLower(word)) {
			cr.Passed = false
			cr.Score += 80
			cr.Actions = append(cr.Actions, risk.ActionReview)
			cr.Events = append(cr.Events, event(rc, "sensitive_content_hit", levelHigh, 80, risk.ActionReview, "sensitive word hit"))
			return cr, nil
		}
	}
	if rc.UID > 0 {
		n, err := c.Store.Incr(ctx, "security:comment:uid:"+uidString(rc.UID), ttlOrDefault(c.Window, time.Minute))
		if err != nil {
			return nil, err
		}
		if n > c.UIDLimit {
			return commentLimited(rc, "comment_review_required", "uid comment limit reached"), nil
		}
	}
	if rc.IP != "" {
		n, err := c.Store.Incr(ctx, "security:comment:ip:"+rc.IP, ttlOrDefault(c.Window, time.Minute))
		if err != nil {
			return nil, err
		}
		if n > c.IPLimit {
			return commentLimited(rc, "comment_spam", "ip comment limit reached"), nil
		}
	}
	if rc.DeviceID != "" {
		n, err := c.Store.Incr(ctx, "security:comment:device:"+rc.DeviceID, ttlOrDefault(c.Window, time.Minute))
		if err != nil {
			return nil, err
		}
		if n > c.DeviceLimit {
			return commentLimited(rc, "comment_spam", "device comment limit reached"), nil
		}
	}
	return cr, nil
}

func commentLimited(rc *risk.Context, eventName string, reason string) *risk.CheckResult {
	return &risk.CheckResult{Passed: false, Score: 70, NeedCaptcha: true, Actions: []risk.Action{risk.ActionCaptcha, risk.ActionReview}, Events: []*audit.Event{event(rc, eventName, levelWarn, 70, risk.ActionReview, reason)}}
}
