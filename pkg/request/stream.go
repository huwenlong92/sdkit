package request

import (
	"context"
	"errors"
	"net/http"
)

type Stream struct {
	Request  *http.Request
	Response *http.Response
}

func (s *Stream) Close() error {
	if s == nil || s.Response == nil || s.Response.Body == nil {
		return nil
	}
	return s.Response.Body.Close()
}

func StreamRequest(ctx context.Context, method, target string, opts ...RequestOption) (*Stream, error) {
	return DefaultClient().Stream(ctx, method, target, opts...)
}

func (c *Client) Stream(ctx context.Context, method, target string, opts ...RequestOption) (*Stream, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if c == nil {
		return nil, ErrNilHTTPClient
	}
	cfg, err := c.applyRequestOptions(opts)
	if err != nil {
		return nil, err
	}
	statusValidator := c.cfg.statusValidator
	if cfg.statusValidator != nil {
		statusValidator = cfg.statusValidator
	}
	maxBodyBytes := c.cfg.maxBodyBytes
	if cfg.maxBodyBytes != nil {
		maxBodyBytes = *cfg.maxBodyBytes
	}
	hooks := append([]Hook(nil), c.cfg.hooks...)
	hooks = append(hooks, cfg.hooks...)

	req, err := c.newHTTPRequest(ctx, method, target, cfg)
	if err != nil {
		return nil, &RequestError{Method: method, URL: target, Err: err}
	}
	for _, hook := range hooks {
		if err := hook.BeforeRequest(ctx, req); err != nil {
			return nil, &RequestError{Method: req.Method, URL: req.URL.String(), Err: err}
		}
	}
	raw, err := c.cfg.httpClient.Do(req)
	if err != nil {
		reqErr := &RequestError{Method: req.Method, URL: req.URL.String(), Err: err}
		return nil, runAfterHooks(ctx, hooks, nil, reqErr)
	}
	resp := newResponse(req, raw, nil)
	if !statusValidator(raw.StatusCode) {
		body, readErr := readResponseBody(raw.Body, maxBodyBytes)
		resp.Body = body
		statusErr := error(&StatusError{Response: resp})
		if readErr != nil {
			statusErr = errors.Join(statusErr, &RequestError{Method: req.Method, URL: req.URL.String(), Err: readErr})
		}
		return nil, runAfterHooks(ctx, hooks, resp, statusErr)
	}
	if err := runAfterHooks(ctx, hooks, resp, nil); err != nil {
		_ = raw.Body.Close()
		return nil, err
	}
	return &Stream{Request: req, Response: raw}, nil
}
