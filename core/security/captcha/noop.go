package captcha

import "context"

type NoopProvider struct{}

func (NoopProvider) Name() string { return "noop" }

func (NoopProvider) Verify(ctx context.Context, token string, ip string) error {
	return ctx.Err()
}
