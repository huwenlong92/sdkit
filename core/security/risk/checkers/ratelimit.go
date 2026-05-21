package checkers

import (
	"context"

	"github.com/huwenlong92/sdkit/core/security/risk"
	"github.com/huwenlong92/sdkit/pkg/ratelimit"
)

type RateLimitChecker struct {
	Scene   string
	KeyFunc func(*risk.Context) string
	Limiter ratelimit.Limiter
}

func (c *RateLimitChecker) Name() string { return "ratelimit" }

func (c *RateLimitChecker) Check(ctx context.Context, rc *risk.Context) (*risk.CheckResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.Limiter == nil || c.Scene != "" && rc.Scene != c.Scene {
		return nil, nil
	}
	key := rc.IP
	if c.KeyFunc != nil {
		key = c.KeyFunc(rc)
	}
	if key == "" || c.Limiter.Allow(key) {
		return &risk.CheckResult{Passed: true}, nil
	}
	return &risk.CheckResult{Passed: false, Actions: []risk.Action{risk.ActionDelay}, NeedCaptcha: true}, nil
}
