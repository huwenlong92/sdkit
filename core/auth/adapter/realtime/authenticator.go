package realtime

import (
	"context"
	"net/http"
	"strings"

	"github.com/huwenlong92/sdkit/core/auth"
	corerealtime "github.com/huwenlong92/sdkit/core/realtime"
)

type Authenticator struct {
	Authenticator auth.RequestAuthenticator
}

func From(authenticator auth.RequestAuthenticator) corerealtime.Authenticator {
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
		UserID:   realtimeSubjectKey(identity),
		TenantID: identity.TenantID,
	}, nil
}

func realtimeSubjectKey(identity *auth.Identity) string {
	if identity == nil {
		return ""
	}
	subject := strings.TrimSpace(identity.SubjectKey())
	subjectType := strings.TrimSpace(identity.SubjectType)
	if subject == "" {
		return ""
	}
	if subjectType == "" {
		return subject
	}
	return subjectType + ":" + subject
}
