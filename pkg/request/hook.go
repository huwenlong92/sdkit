package request

import (
	"context"
	"net/http"
)

type Hook interface {
	BeforeRequest(ctx context.Context, req *http.Request) error
	AfterResponse(ctx context.Context, resp *Response, err error) error
}

type HookFunc struct {
	Before func(ctx context.Context, req *http.Request) error
	After  func(ctx context.Context, resp *Response, err error) error
}

func (h HookFunc) BeforeRequest(ctx context.Context, req *http.Request) error {
	if h.Before == nil {
		return nil
	}
	return h.Before(ctx, req)
}

func (h HookFunc) AfterResponse(ctx context.Context, resp *Response, err error) error {
	if h.After == nil {
		return nil
	}
	return h.After(ctx, resp, err)
}
