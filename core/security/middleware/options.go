package middleware

import "github.com/huwenlong92/sdkit/core/ginresponder"

type MiddlewareConfig struct {
	Responder ginresponder.ErrorResponder
}

type MiddlewareOption func(*MiddlewareConfig)

func WithResponder(responder ginresponder.ErrorResponder) MiddlewareOption {
	return func(cfg *MiddlewareConfig) {
		cfg.Responder = responder
	}
}

func newMiddlewareConfig(opts ...MiddlewareOption) *MiddlewareConfig {
	cfg := &MiddlewareConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}
