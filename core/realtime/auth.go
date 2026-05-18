package realtime

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/huwenlong92/sdkit/core/auth"

	jwt "github.com/golang-jwt/jwt/v5"
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

type JWTAuthenticator struct {
	Secret      string
	TokenQuery  string
	AllowCookie bool
}

func (a JWTAuthenticator) Authenticate(_ context.Context, r *http.Request) (*AuthResult, error) {
	if a.AllowCookie {
		if result, ok := a.authenticateSession(r); ok {
			return result, nil
		}
	}

	tokenStr := ""
	if a.TokenQuery != "" {
		tokenStr = r.URL.Query().Get(a.TokenQuery)
	}
	if tokenStr == "" {
		authHeader := r.Header.Get("Authorization")
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			tokenStr = parts[1]
		}
	}
	if tokenStr == "" {
		return nil, ErrUnauthorized
	}

	token, err := jwt.ParseWithClaims(tokenStr, &auth.JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(a.Secret), nil
	})
	if err != nil {
		return nil, ErrUnauthorized
	}
	claims, ok := token.Claims.(*auth.JWTClaims)
	if !ok || !token.Valid || claims.SubjectID == 0 {
		return nil, ErrUnauthorized
	}
	return &AuthResult{UserID: strconv.FormatInt(claims.SubjectID, 10)}, nil
}

func (a JWTAuthenticator) authenticateSession(r *http.Request) (result *AuthResult, ok bool) {
	return nil, false
}

type AllowAnonymousAuthenticator struct{}

func (AllowAnonymousAuthenticator) Authenticate(_ context.Context, _ *http.Request) (*AuthResult, error) {
	return &AuthResult{}, nil
}
