package captcha

import (
	"context"
	"time"
)

type Kind string

const (
	KindImage  Kind = "image"
	KindSlider Kind = "slider"
)

type Challenge struct {
	ID       string         `json:"id"`
	Kind     Kind           `json:"kind"`
	Image    string         `json:"image,omitempty"`
	Audio    string         `json:"audio,omitempty"`
	Payload  map[string]any `json:"payload,omitempty"`
	ExpireAt time.Time      `json:"expire_at,omitempty"`
}

type GenerateOptions struct {
	TTL time.Duration
}

type Provider interface {
	Name() string
	Kind() Kind
	Generate(ctx context.Context, opts GenerateOptions) (*Challenge, error)
	Verify(ctx context.Context, id string, answer string) error
}
