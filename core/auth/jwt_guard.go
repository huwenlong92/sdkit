package auth

import (
	"context"
)

type JWTConfig struct {
	Secret string `mapstructure:"secret" yaml:"secret"`
	Issuer string `mapstructure:"issuer" yaml:"issuer"`
	Expire int    `mapstructure:"expire" yaml:"expire"`
}

type JWTGuard struct {
	secret string
	issuer string
	expire int
}

func NewJWTGuard(cfg *JWTConfig) *JWTGuard {
	if cfg == nil {
		cfg = &JWTConfig{}
	}
	return &JWTGuard{secret: cfg.Secret, issuer: cfg.Issuer, expire: cfg.Expire}
}

func (g *JWTGuard) Mode() Mode {
	return ModeJWT
}

func (g *JWTGuard) Login(ctx context.Context, identity *Identity) (*LoginResult, error) {
	if identity == nil {
		return nil, ErrUnauthorized
	}
	token, err := generateToken(g.secret, g.issuer, g.expire, identity)
	if err != nil {
		return nil, err
	}
	return &LoginResult{Mode: ModeJWT, Token: token, Identity: identity}, nil
}

func (g *JWTGuard) Logout(ctx context.Context, credential string) error {
	return nil
}

func (g *JWTGuard) Authenticate(ctx context.Context, credential string) (*Identity, error) {
	if credential == "" {
		return nil, ErrUnauthorized
	}
	claims, err := parseTokenWithSecret(credential, g.secret)
	if err != nil {
		return nil, err
	}
	return identityFromClaims(claims), nil
}
