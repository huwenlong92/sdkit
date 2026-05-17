// Package auth provides service-agnostic authentication orchestration.
package auth

import (
	"context"
)

type Auth struct {
	hooks Hooks
	guard Guard
}

// Default is the global auth manager injected by bootstrap or service startup.
var Default *Auth

func NewWithGuard(guard Guard, hooks Hooks) *Auth {
	return &Auth{guard: guard, hooks: hooks}
}

func (a *Auth) Mode() Mode {
	if a == nil || a.guard == nil {
		return ""
	}
	return a.guard.Mode()
}

func (a *Auth) Guard() Guard {
	if a == nil {
		return nil
	}
	return a.guard
}

func (a *Auth) LoginByCredentials(ctx context.Context, credentials Credentials) (*LoginResult, error) {
	if a == nil || a.hooks == nil || a.guard == nil {
		return nil, ErrUnauthorized
	}

	user, err := a.hooks.FindUser(ctx, credentials.Username)
	if err != nil {
		a.hooks.AfterLoginFailed(ctx, credentials.Username, err)
		return nil, err
	}
	if err := a.hooks.CheckPassword(ctx, user, credentials.Password); err != nil {
		a.hooks.AfterLoginFailed(ctx, credentials.Username, err)
		return nil, err
	}
	identity, err := a.hooks.BuildIdentity(ctx, user)
	if err != nil {
		a.hooks.AfterLoginFailed(ctx, credentials.Username, err)
		return nil, err
	}
	result, err := a.guard.Login(ctx, identity)
	if err != nil {
		a.hooks.AfterLoginFailed(ctx, credentials.Username, err)
		return nil, err
	}
	if err := a.hooks.AfterLogin(ctx, user, identity); err != nil {
		return nil, err
	}
	return result, nil
}

func (a *Auth) Logout(ctx context.Context, credential string) error {
	if a == nil || a.guard == nil {
		return nil
	}
	return a.guard.Logout(ctx, credential)
}

func (a *Auth) Authenticate(ctx context.Context, credential string) (*Identity, error) {
	if a == nil || a.guard == nil {
		return nil, ErrUnauthorized
	}
	return a.guard.Authenticate(ctx, credential)
}
