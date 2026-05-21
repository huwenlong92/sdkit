package strategy

import (
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/pkg/ratelimit/store"
)

func TestTokenBucket(t *testing.T) {
	limiter := NewTokenBucket(0.000001, 2, store.NewMemoryStore())
	if !limiter.Allow("ip:1") {
		t.Fatalf("first request should pass")
	}
	if !limiter.Allow("ip:1") {
		t.Fatalf("second request should pass")
	}
	if limiter.Allow("ip:1") {
		t.Fatalf("third request should be limited")
	}
}

func TestSlidingWindow(t *testing.T) {
	limiter := NewSlidingWindow(time.Minute, 2, store.NewMemoryStore())
	if !limiter.Allow("ip:1") {
		t.Fatalf("first request should pass")
	}
	if !limiter.Allow("ip:1") {
		t.Fatalf("second request should pass")
	}
	if limiter.Allow("ip:1") {
		t.Fatalf("third request should be limited")
	}
}

func TestFixedWindow(t *testing.T) {
	limiter := NewFixedWindow(time.Minute, 1, store.NewMemoryStore())
	if !limiter.Allow("ip:1") {
		t.Fatalf("first request should pass")
	}
	if limiter.Allow("ip:1") {
		t.Fatalf("second request should be limited")
	}
}

func TestLeakyBucket(t *testing.T) {
	limiter := NewLeakyBucket(1, 1)
	if !limiter.Allow("ip:1") {
		t.Fatalf("first request should pass")
	}
	if limiter.Allow("ip:1") {
		t.Fatalf("second request should be limited")
	}
}
