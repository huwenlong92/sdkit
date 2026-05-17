package auth

import "context"

type Hooks interface {
	FindUser(ctx context.Context, username string) (any, error)
	CheckPassword(ctx context.Context, user any, password string) error
	BuildIdentity(ctx context.Context, user any) (*Identity, error)
	AfterLogin(ctx context.Context, user any, identity *Identity) error
	AfterLoginFailed(ctx context.Context, username string, err error)
}

type HookFuncs struct {
	FindUserFunc         func(ctx context.Context, username string) (any, error)
	CheckPasswordFunc    func(ctx context.Context, user any, password string) error
	BuildIdentityFunc    func(ctx context.Context, user any) (*Identity, error)
	AfterLoginFunc       func(ctx context.Context, user any, identity *Identity) error
	AfterLoginFailedFunc func(ctx context.Context, username string, err error)
}

func (h HookFuncs) FindUser(ctx context.Context, username string) (any, error) {
	if h.FindUserFunc == nil {
		return nil, ErrHookNotImplemented
	}
	return h.FindUserFunc(ctx, username)
}

func (h HookFuncs) CheckPassword(ctx context.Context, user any, password string) error {
	if h.CheckPasswordFunc == nil {
		return ErrHookNotImplemented
	}
	return h.CheckPasswordFunc(ctx, user, password)
}

func (h HookFuncs) BuildIdentity(ctx context.Context, user any) (*Identity, error) {
	if h.BuildIdentityFunc == nil {
		return nil, ErrHookNotImplemented
	}
	return h.BuildIdentityFunc(ctx, user)
}

func (h HookFuncs) AfterLogin(ctx context.Context, user any, identity *Identity) error {
	if h.AfterLoginFunc == nil {
		return nil
	}
	return h.AfterLoginFunc(ctx, user, identity)
}

func (h HookFuncs) AfterLoginFailed(ctx context.Context, username string, err error) {
	if h.AfterLoginFailedFunc != nil {
		h.AfterLoginFailedFunc(ctx, username, err)
	}
}
