package realtime

import (
	"context"
	"net/http"

	coreauth "github.com/huwenlong92/sdkit/core/auth"
	corerealtime "github.com/huwenlong92/sdkit/core/realtime"
)

type Authenticator struct {
	Authenticator coreauth.RequestAuthenticator
}

func From(authenticator coreauth.RequestAuthenticator) corerealtime.Authenticator {
	return Authenticator{Authenticator: authenticator}
}

func (a Authenticator) Authenticate(ctx context.Context, r *http.Request) (*corerealtime.AuthResult, error) {
	if a.Authenticator == nil {
		return nil, corerealtime.ErrUnauthorized
	}
	identity, err := a.Authenticator.AuthenticateRequest(ctx, r)
	if err != nil {
		return nil, corerealtime.ErrUnauthorized
	}
	if identity == nil || !identity.Authenticated() {
		return &corerealtime.AuthResult{}, nil
	}
	return &corerealtime.AuthResult{
		UserID:   identity.SubjectKey(),
		TenantID: identity.TenantID,
	}, nil
}
