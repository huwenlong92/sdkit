package captcha

import "context"

type NoopProvider struct{}

func (NoopProvider) Name() string { return "noop" }

func (NoopProvider) Kind() Kind { return KindImage }

func (NoopProvider) Generate(ctx context.Context, opts GenerateOptions) (*Challenge, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Challenge{ID: "noop", Kind: KindImage}, nil
}

func (NoopProvider) Verify(ctx context.Context, id string, answer string) error {
	return ctx.Err()
}
