package captcha

import (
	"context"
	"time"

	"github.com/mojocn/base64Captcha"
)

type Base64Option func(*base64Options)

type base64Options struct {
	height    int
	width     int
	length    int
	maxSkew   float64
	dotCount  int
	ttl       time.Duration
	store     base64Captcha.Store
	driver    base64Captcha.Driver
	name      string
	imageKind Kind
}

type Base64Provider struct {
	captcha *base64Captcha.Captcha
	ttl     time.Duration
	name    string
	kind    Kind
}

func NewBase64Provider(opts ...Base64Option) *Base64Provider {
	options := base64Options{
		height:    45,
		width:     140,
		length:    5,
		maxSkew:   0.3,
		dotCount:  10,
		ttl:       5 * time.Minute,
		name:      "base64",
		imageKind: KindImage,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	driver := options.driver
	if driver == nil {
		driver = base64Captcha.NewDriverDigit(options.height, options.width, options.length, options.maxSkew, options.dotCount)
	}
	store := options.store
	if store == nil {
		store = base64Captcha.DefaultMemStore
	}
	return &Base64Provider{
		captcha: base64Captcha.NewCaptcha(driver, store),
		ttl:     options.ttl,
		name:    options.name,
		kind:    options.imageKind,
	}
}

func WithBase64Store(store base64Captcha.Store) Base64Option {
	return func(o *base64Options) {
		o.store = store
	}
}

func WithBase64Driver(driver base64Captcha.Driver) Base64Option {
	return func(o *base64Options) {
		o.driver = driver
	}
}

func WithBase64DigitSize(height, width, length int) Base64Option {
	return func(o *base64Options) {
		o.height = height
		o.width = width
		o.length = length
	}
}

func WithBase64TTL(ttl time.Duration) Base64Option {
	return func(o *base64Options) {
		o.ttl = ttl
	}
}

func (p *Base64Provider) Name() string { return p.name }

func (p *Base64Provider) Kind() Kind { return p.kind }

func (p *Base64Provider) Generate(ctx context.Context, opts GenerateOptions) (*Challenge, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id, image, _, err := p.captcha.Generate()
	if err != nil {
		return nil, err
	}
	ttl := p.ttl
	if opts.TTL > 0 {
		ttl = opts.TTL
	}
	return &Challenge{ID: id, Kind: p.kind, Image: image, ExpireAt: time.Now().Add(ttl)}, nil
}

func (p *Base64Provider) Verify(ctx context.Context, id string, answer string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !p.captcha.Verify(id, answer, true) {
		return ErrInvalidToken
	}
	return nil
}
