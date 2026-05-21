package ratelimit

import (
	"context"
	"sync"
	"time"
)

type Limiter interface {
	Allow(key string) bool
	AllowN(key string, n int) bool
}

type ContextLimiter interface {
	Limiter
	AllowContext(ctx context.Context, key string) bool
	AllowNContext(ctx context.Context, key string, n int) bool
}

type Option struct {
	Rate   float64
	Burst  int
	Window time.Duration
	Limit  int
}

type PerKey struct {
	inner    func() Limiter
	mu       sync.RWMutex
	limiters map[string]Limiter
}

func NewPerKey(inner func() Limiter) *PerKey {
	return &PerKey{inner: inner, limiters: make(map[string]Limiter)}
}

func (p *PerKey) Allow(key string) bool { return p.AllowN(key, 1) }

func (p *PerKey) AllowN(key string, n int) bool {
	return p.AllowNContext(context.Background(), key, n)
}

func (p *PerKey) AllowContext(ctx context.Context, key string) bool {
	return p.AllowNContext(ctx, key, 1)
}

func (p *PerKey) AllowNContext(ctx context.Context, key string, n int) bool {
	p.mu.RLock()
	l, ok := p.limiters[key]
	p.mu.RUnlock()

	if !ok {
		l = p.inner()
		p.mu.Lock()
		p.limiters[key] = l
		p.mu.Unlock()
	}
	if ctxLimiter, ok := l.(ContextLimiter); ok {
		return ctxLimiter.AllowNContext(ctx, key, n)
	}
	return l.AllowN(key, n)
}
