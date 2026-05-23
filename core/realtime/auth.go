package realtime

import (
	"context"
	"errors"
	"net/http"
)

var ErrUnauthorized = errors.New("realtime unauthorized")

type AuthResult struct {
	UserID   string
	TenantID int64
}

func (r *AuthResult) Identity() *Identity {
	return IdentityFromAuthResult(r)
}

type Authenticator interface {
	Authenticate(ctx context.Context, r *http.Request) (*AuthResult, error)
}

type AllowAnonymousAuthenticator struct{}

func (AllowAnonymousAuthenticator) Authenticate(_ context.Context, _ *http.Request) (*AuthResult, error) {
	return &AuthResult{}, nil
}
