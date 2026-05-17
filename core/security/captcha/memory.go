package captcha

import (
	"context"
	"errors"
	"sync"
)

var ErrInvalidToken = errors.New("security captcha: invalid token")

type MemoryProvider struct {
	mu     sync.Mutex
	tokens map[string]struct{}
}

func NewMemoryProvider(tokens ...string) *MemoryProvider {
	p := &MemoryProvider{tokens: make(map[string]struct{})}
	for _, token := range tokens {
		p.tokens[token] = struct{}{}
	}
	return p
}

func (p *MemoryProvider) Name() string { return "memory" }

func (p *MemoryProvider) Add(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tokens[token] = struct{}{}
}

func (p *MemoryProvider) Verify(ctx context.Context, token string, ip string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.tokens[token]; !ok {
		return ErrInvalidToken
	}
	delete(p.tokens, token)
	return nil
}
