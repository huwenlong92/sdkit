package sms

import (
	"context"
	"time"
)

type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error)
}

type RateLimitRule struct {
	Key    func(Request) string
	Limit  int64
	Window time.Duration
}

func PhoneKey(req Request) string {
	if len(req.To) == 0 {
		return ""
	}
	return "sms:phone:" + req.To[0]
}

func RateLimitMiddleware(limiter RateLimiter, rule RateLimitRule) Middleware {
	return func(next Sender) Sender {
		return SenderFunc(func(ctx context.Context, req Request) (*SendResult, error) {
			if limiter == nil || rule.Limit <= 0 || rule.Window <= 0 {
				return next.Send(ctx, req)
			}
			keyFunc := rule.Key
			if keyFunc == nil {
				keyFunc = PhoneKey
			}
			key := keyFunc(req)
			if key == "" {
				return next.Send(ctx, req)
			}
			allowed, err := limiter.Allow(ctx, key, rule.Limit, rule.Window)
			if err != nil {
				return nil, err
			}
			if !allowed {
				return nil, ErrRateLimited
			}
			return next.Send(ctx, req)
		})
	}
}
