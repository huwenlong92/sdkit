package captcha

import (
	"context"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/core/security/captcha/store"
	"github.com/huwenlong92/sdkit/pkg/security/token"
)

const defaultClientTokenAnswer = "passed"

type ClientTokenProvider struct {
	kind  Kind
	name  string
	store store.Store
	ttl   time.Duration
}

func newClientTokenProvider(kind Kind, name string, store store.Store, ttl time.Duration) *ClientTokenProvider {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &ClientTokenProvider{
		kind:  kind,
		name:  strings.TrimSpace(name),
		store: store,
		ttl:   ttl,
	}
}

func NewClientSliderProvider(store store.Store, ttl time.Duration) *ClientTokenProvider {
	return newClientTokenProvider(KindClientSlider, "client-slider", store, ttl)
}

func NewClientClickProvider(store store.Store, ttl time.Duration) *ClientTokenProvider {
	return newClientTokenProvider(KindClientClick, "client-click", store, ttl)
}

func (p *ClientTokenProvider) Name() string { return p.name }

func (p *ClientTokenProvider) Kind() Kind { return p.kind }

func (p *ClientTokenProvider) Generate(ctx context.Context, opts GenerateOptions) (*Challenge, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p == nil || p.store == nil {
		return nil, ErrInvalidToken
	}
	id, err := token.RandomToken(16)
	if err != nil {
		return nil, err
	}
	ttl := p.ttl
	if opts.TTL > 0 {
		ttl = opts.TTL
	}
	if err := p.store.Set(ctx, id, defaultClientTokenAnswer, ttl); err != nil {
		return nil, err
	}
	return &Challenge{
		ID:       id,
		Kind:     p.kind,
		ExpireAt: time.Now().Add(ttl),
	}, nil
}

func (p *ClientTokenProvider) Verify(ctx context.Context, id string, answer string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p == nil || p.store == nil {
		return ErrInvalidToken
	}
	value, ok, err := p.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if !ok || value != defaultClientTokenAnswer || strings.TrimSpace(answer) != defaultClientTokenAnswer {
		return ErrInvalidToken
	}
	return p.store.Delete(ctx, id)
}
