package captcha

import (
	"context"
	"errors"
	"sync"
	"time"

	securitytoken "github.com/huwenlong92/sdkit/pkg/security/token"
)

var (
	ErrInvalidToken     = errors.New("security captcha: invalid token")
	ErrProviderNotFound = errors.New("security captcha: provider not found")
)

type MemoryProvider struct {
	mu     sync.Mutex
	tokens map[string]string
}

func NewMemoryProvider(tokens ...string) *MemoryProvider {
	p := &MemoryProvider{tokens: make(map[string]string)}
	for _, token := range tokens {
		p.tokens[token] = token
	}
	return p
}

func (p *MemoryProvider) Name() string { return "memory" }

func (p *MemoryProvider) Kind() Kind { return KindImage }

func (p *MemoryProvider) Generate(ctx context.Context, opts GenerateOptions) (*Challenge, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	token, err := securitytoken.VerifyCode(6)
	if err != nil {
		return nil, err
	}
	id, err := securitytoken.RandomToken(16)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.tokens[id] = token
	p.mu.Unlock()

	challenge := &Challenge{ID: id, Kind: p.Kind()}
	if opts.TTL > 0 {
		challenge.ExpireAt = time.Now().Add(opts.TTL)
	}
	return challenge, nil
}

func (p *MemoryProvider) Add(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tokens[token] = token
}

func (p *MemoryProvider) Verify(ctx context.Context, id string, answer string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if want, ok := p.tokens[id]; ok {
		if want != answer {
			return ErrInvalidToken
		}
		delete(p.tokens, id)
		return nil
	}
	if _, ok := p.tokens[answer]; !ok {
		return ErrInvalidToken
	}
	delete(p.tokens, answer)
	return nil
}
