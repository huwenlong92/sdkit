package auth

import (
	"context"
	"net/http"
	"strings"
)

const (
	CredentialBearer  = "bearer"
	CredentialQuery   = "query"
	CredentialCookie  = "cookie"
	CredentialSession = "session"
)

type Credential struct {
	Type     string
	Value    string
	Source   string
	Provider string
}

type Extractor interface {
	Extract(*http.Request) (Credential, bool)
}

type ExtractorFunc func(*http.Request) (Credential, bool)

func (f ExtractorFunc) Extract(r *http.Request) (Credential, bool) {
	if f == nil {
		return Credential{}, false
	}
	return f(r)
}

type RequestAuthenticator interface {
	AuthenticateRequest(ctx context.Context, r *http.Request) (*Identity, error)
}

type RequestAuthenticatorFunc func(context.Context, *http.Request) (*Identity, error)

func (f RequestAuthenticatorFunc) AuthenticateRequest(ctx context.Context, r *http.Request) (*Identity, error) {
	if f == nil {
		return nil, ErrUnauthorized
	}
	return f(ctx, r)
}

func FirstExtractor(extractors ...Extractor) Extractor {
	return ExtractorFunc(func(r *http.Request) (Credential, bool) {
		for _, extractor := range extractors {
			if extractor == nil {
				continue
			}
			credential, ok := extractor.Extract(r)
			if ok && strings.TrimSpace(credential.Value) != "" {
				return credential, true
			}
		}
		return Credential{}, false
	})
}

func BearerTokenExtractor() Extractor {
	return ExtractorFunc(func(r *http.Request) (Credential, bool) {
		if r == nil {
			return Credential{}, false
		}
		parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return Credential{}, false
		}
		token := strings.TrimSpace(parts[1])
		if token == "" {
			return Credential{}, false
		}
		return Credential{Type: CredentialBearer, Value: token, Source: "authorization"}, true
	})
}

func QueryTokenExtractor(name string) Extractor {
	return ExtractorFunc(func(r *http.Request) (Credential, bool) {
		if r == nil {
			return Credential{}, false
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return Credential{}, false
		}
		token := strings.TrimSpace(r.URL.Query().Get(name))
		if token == "" {
			return Credential{}, false
		}
		return Credential{Type: CredentialQuery, Value: token, Source: name}, true
	})
}

func CookieTokenExtractor(names ...string) Extractor {
	return ExtractorFunc(func(r *http.Request) (Credential, bool) {
		if r == nil {
			return Credential{}, false
		}
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			cookie, err := r.Cookie(name)
			if err != nil || cookie == nil {
				continue
			}
			token := strings.TrimSpace(cookie.Value)
			if token == "" {
				continue
			}
			return Credential{Type: CredentialCookie, Value: token, Source: name}, true
		}
		return Credential{}, false
	})
}
