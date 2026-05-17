package captcha

import "context"

type Provider interface {
	Name() string
	Verify(ctx context.Context, token string, ip string) error
}
