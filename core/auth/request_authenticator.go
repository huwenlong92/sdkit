package auth

import (
	"context"
	"errors"
	"net/http"
)

type JWTAuthenticator struct {
	cfg       JWTConfig
	extractor Extractor
	provider  string
}

type JWTAuthenticatorOption func(*JWTAuthenticator)

func NewJWTAuthenticator(cfg *JWTConfig, opts ...JWTAuthenticatorOption) *JWTAuthenticator {
	authenticator := &JWTAuthenticator{
		cfg:       normalizeJWTConfig(cfg),
		extractor: BearerTokenExtractor(),
		provider:  "jwt",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(authenticator)
		}
	}
	return authenticator
}

func WithJWTExtractor(extractor Extractor) JWTAuthenticatorOption {
	return func(a *JWTAuthenticator) {
		if extractor != nil {
			a.extractor = extractor
		}
	}
}

func WithJWTProvider(provider string) JWTAuthenticatorOption {
	return func(a *JWTAuthenticator) {
		if provider != "" {
			a.provider = provider
		}
	}
}

func (a *JWTAuthenticator) AuthenticateRequest(ctx context.Context, r *http.Request) (*Identity, error) {
	if a == nil || a.extractor == nil {
		return nil, ErrUnauthorized
	}
	credential, ok := a.extractor.Extract(r)
	if !ok {
		return nil, ErrUnauthorized
	}
	claims, err := parseTokenWithSecret(credential.Value, a.cfg.Secret)
	if err != nil {
		return nil, err
	}
	identity := identityFromClaims(claims)
	enrichIdentity(identity, MethodJWT, providerName(a.provider, credential.Provider))
	identity.TokenID = credential.Source
	return identity, nil
}

func (a *JWTAuthenticator) Login(ctx context.Context, identity *Identity) (*LoginResult, error) {
	if a == nil {
		return nil, ErrUnauthorized
	}
	if identity == nil {
		return nil, ErrUnauthorized
	}
	token, err := generateToken(a.cfg.Secret, a.cfg.Issuer, a.cfg.Expire, identity)
	if err != nil {
		return nil, err
	}
	enrichIdentity(identity, MethodJWT, a.provider)
	return &LoginResult{Token: token, Identity: identity}, nil
}

func normalizeJWTConfig(cfg *JWTConfig) JWTConfig {
	if cfg == nil {
		return JWTConfig{}
	}
	return *cfg
}

type SessionReader interface {
	ReadSession(ctx context.Context, r *http.Request, key string) (any, bool, error)
}

type SessionMapper func(ctx context.Context, raw any) (*Identity, error)
type IdentityHook func(ctx context.Context, identity *Identity, raw any) error
type IdentityFailureHook func(ctx context.Context, r *http.Request, err error) error
type IdentityValidator = IdentityHook
type IdentityRefresher = IdentityHook

type SessionAuthenticator struct {
	Provider   string
	Key        string
	Reader     SessionReader
	Mapper     SessionMapper
	Validator  IdentityValidator
	Validators []IdentityHook
	Refresher  IdentityRefresher
	Refreshers []IdentityHook
	Failure    IdentityFailureHook
	Failures   []IdentityFailureHook
}

func NewSessionAuthenticator(opts SessionAuthenticator) *SessionAuthenticator {
	return &SessionAuthenticator{
		Provider:   opts.Provider,
		Key:        opts.Key,
		Reader:     opts.Reader,
		Mapper:     opts.Mapper,
		Validator:  opts.Validator,
		Validators: append([]IdentityHook(nil), opts.Validators...),
		Refresher:  opts.Refresher,
		Refreshers: append([]IdentityHook(nil), opts.Refreshers...),
		Failure:    opts.Failure,
		Failures:   append([]IdentityFailureHook(nil), opts.Failures...),
	}
}

func (a *SessionAuthenticator) AuthenticateRequest(ctx context.Context, r *http.Request) (*Identity, error) {
	if a == nil || a.Reader == nil || a.Mapper == nil || a.Key == "" {
		return nil, a.fail(ctx, r, ErrUnauthorized)
	}
	raw, ok, err := a.Reader.ReadSession(ctx, r, a.Key)
	if err != nil {
		return nil, a.fail(ctx, r, err)
	}
	if !ok || raw == nil {
		return nil, a.fail(ctx, r, ErrUnauthorized)
	}
	identity, err := a.Mapper(ctx, raw)
	if err != nil {
		return nil, a.fail(ctx, r, err)
	}
	if identity == nil || !identity.Authenticated() {
		return nil, a.fail(ctx, r, ErrUnauthorized)
	}
	enrichIdentity(identity, MethodSession, providerName(a.Provider, "session"))
	if err := runIdentityHooks(ctx, identity, raw, a.Validator, a.Validators); err != nil {
		return nil, a.fail(ctx, r, err)
	}
	if err := runIdentityHooks(ctx, identity, raw, a.Refresher, a.Refreshers); err != nil {
		return nil, err
	}
	return identity, nil
}

func (a *SessionAuthenticator) fail(ctx context.Context, r *http.Request, err error) error {
	if err == nil {
		err = ErrUnauthorized
	}
	if a == nil {
		return err
	}
	hooks := make([]IdentityFailureHook, 0, len(a.Failures)+1)
	if a.Failure != nil {
		hooks = append(hooks, a.Failure)
	}
	hooks = append(hooks, a.Failures...)
	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		if hookErr := hook(ctx, r, err); hookErr != nil {
			return hookErr
		}
	}
	return err
}

func runIdentityHooks(ctx context.Context, identity *Identity, raw any, first IdentityHook, rest []IdentityHook) error {
	if first != nil {
		if err := first(ctx, identity, raw); err != nil {
			return err
		}
	}
	for _, hook := range rest {
		if hook == nil {
			continue
		}
		if err := hook(ctx, identity, raw); err != nil {
			return err
		}
	}
	return nil
}

type ChainAuthenticator struct {
	authenticators []RequestAuthenticator
}

func NewChainAuthenticator(authenticators ...RequestAuthenticator) *ChainAuthenticator {
	return &ChainAuthenticator{authenticators: authenticators}
}

func (a *ChainAuthenticator) AuthenticateRequest(ctx context.Context, r *http.Request) (*Identity, error) {
	if a == nil {
		return nil, ErrUnauthorized
	}
	var lastErr error
	for _, authenticator := range a.authenticators {
		if authenticator == nil {
			continue
		}
		identity, err := authenticator.AuthenticateRequest(ctx, r)
		if err == nil && identity != nil {
			return identity, nil
		}
		if !errors.Is(err, ErrUnauthorized) {
			return nil, err
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrUnauthorized
}

type AnonymousAuthenticator struct{}

func (AnonymousAuthenticator) AuthenticateRequest(context.Context, *http.Request) (*Identity, error) {
	return &Identity{Method: MethodAnonymous, Provider: MethodAnonymous}, nil
}

func enrichIdentity(identity *Identity, method string, provider string) {
	if identity == nil {
		return
	}
	if identity.Method == "" {
		identity.Method = method
	}
	if identity.Provider == "" {
		identity.Provider = provider
	}
}

func providerName(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
