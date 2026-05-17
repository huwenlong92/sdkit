package auth

import "context"

type Guard interface {
	Mode() Mode
	Login(ctx context.Context, identity *Identity) (*LoginResult, error)
	Logout(ctx context.Context, credential string) error
	Authenticate(ctx context.Context, credential string) (*Identity, error)
}
