package ratelimit

import "testing"

type staticLimiter struct {
	allowed bool
}

func (l staticLimiter) Allow(key string) bool { return l.allowed }
func (l staticLimiter) AllowN(key string, n int) bool {
	return l.allowed
}

func TestPerKey(t *testing.T) {
	created := 0
	limiter := NewPerKey(func() Limiter {
		created++
		return staticLimiter{allowed: true}
	})

	if !limiter.Allow("a") || !limiter.Allow("a") || !limiter.Allow("b") {
		t.Fatalf("expected requests to pass")
	}
	if created != 2 {
		t.Fatalf("created = %d, want 2", created)
	}
}
