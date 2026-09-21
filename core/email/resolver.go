package email

import "context"

// ProviderResolver 按稳定账号名解析当前可用的邮件 Provider 配置。
// 实现方可以从数据库或配置中心读取，但不得在错误和日志中泄露密码。
type ProviderResolver interface {
	Resolve(ctx context.Context, name string) (ProviderConfig, error)
}

// ProviderResolverFunc 将函数适配为 ProviderResolver。
type ProviderResolverFunc func(ctx context.Context, name string) (ProviderConfig, error)

func (fn ProviderResolverFunc) Resolve(ctx context.Context, name string) (ProviderConfig, error) {
	if fn == nil {
		return ProviderConfig{}, ErrNotConfigured
	}
	return fn(ctx, name)
}
