package request

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

type Client struct {
	cfg clientConfig
}

var defaultClient = func() *Client {
	client, err := NewClient()
	if err != nil {
		return &Client{cfg: defaultClientConfig()}
	}
	return client
}()

func NewClient(opts ...ClientOption) (*Client, error) {
	cfg := defaultClientConfig()
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	if cfg.httpClient == nil {
		return nil, ErrNilHTTPClient
	}
	if cfg.headers == nil {
		cfg.headers = make(http.Header)
	}
	if cfg.query == nil {
		cfg.query = make(url.Values)
	}
	cfg.retry = normalizeRetry(cfg.retry)
	if cfg.statusValidator == nil {
		cfg.statusValidator = defaultStatusValidator
	}
	return &Client{cfg: cfg}, nil
}

func DefaultClient() *Client {
	return defaultClient
}

func Do(ctx context.Context, method, target string, opts ...RequestOption) (*Response, error) {
	return DefaultClient().Do(ctx, method, target, opts...)
}

func Get(ctx context.Context, target string, opts ...RequestOption) (*Response, error) {
	return DefaultClient().Get(ctx, target, opts...)
}

func Post(ctx context.Context, target string, opts ...RequestOption) (*Response, error) {
	return DefaultClient().Post(ctx, target, opts...)
}

func Put(ctx context.Context, target string, opts ...RequestOption) (*Response, error) {
	return DefaultClient().Put(ctx, target, opts...)
}

func Patch(ctx context.Context, target string, opts ...RequestOption) (*Response, error) {
	return DefaultClient().Patch(ctx, target, opts...)
}

func Delete(ctx context.Context, target string, opts ...RequestOption) (*Response, error) {
	return DefaultClient().Delete(ctx, target, opts...)
}

func Head(ctx context.Context, target string, opts ...RequestOption) (*Response, error) {
	return DefaultClient().Head(ctx, target, opts...)
}

func Options(ctx context.Context, target string, opts ...RequestOption) (*Response, error) {
	return DefaultClient().Options(ctx, target, opts...)
}

func (c *Client) Get(ctx context.Context, target string, opts ...RequestOption) (*Response, error) {
	return c.Do(ctx, http.MethodGet, target, opts...)
}

func (c *Client) Post(ctx context.Context, target string, opts ...RequestOption) (*Response, error) {
	return c.Do(ctx, http.MethodPost, target, opts...)
}

func (c *Client) Put(ctx context.Context, target string, opts ...RequestOption) (*Response, error) {
	return c.Do(ctx, http.MethodPut, target, opts...)
}

func (c *Client) Patch(ctx context.Context, target string, opts ...RequestOption) (*Response, error) {
	return c.Do(ctx, http.MethodPatch, target, opts...)
}

func (c *Client) Delete(ctx context.Context, target string, opts ...RequestOption) (*Response, error) {
	return c.Do(ctx, http.MethodDelete, target, opts...)
}

func (c *Client) Head(ctx context.Context, target string, opts ...RequestOption) (*Response, error) {
	return c.Do(ctx, http.MethodHead, target, opts...)
}

func (c *Client) Options(ctx context.Context, target string, opts ...RequestOption) (*Response, error) {
	return c.Do(ctx, http.MethodOptions, target, opts...)
}

func (c *Client) Do(ctx context.Context, method, target string, opts ...RequestOption) (*Response, error) {
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
	if cfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.timeout)
		defer cancel()
	}

	retry := c.cfg.retry
	if cfg.retry != nil {
		retry = *cfg.retry
	}
	retry = normalizeRetry(retry)
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

	var lastResp *Response
	var lastErr error
	for attempt := 1; attempt <= retry.MaxAttempts; attempt++ {
		req, err := c.newHTTPRequest(ctx, method, target, cfg)
		if err != nil {
			return lastResp, &RequestError{Method: method, URL: target, Err: err}
		}
		resp, err := c.doOnce(ctx, req, maxBodyBytes, hooks)
		lastResp, lastErr = resp, err
		if !shouldRetry(ctx, retry, method, attempt, resp, err) {
			break
		}
		if err := waitRetry(ctx, retryWait(retry, attempt)); err != nil {
			return lastResp, &RequestError{Method: method, URL: req.URL.String(), Err: err}
		}
	}
	if lastErr != nil {
		return lastResp, lastErr
	}
	if lastResp != nil && !statusValidator(lastResp.StatusCode) {
		return lastResp, &StatusError{Response: lastResp}
	}
	if cfg.decodeJSON != nil && lastResp != nil {
		if err := lastResp.DecodeJSON(cfg.decodeJSON); err != nil {
			return lastResp, err
		}
	}
	return lastResp, nil
}

func (c *Client) applyRequestOptions(opts []RequestOption) (requestConfig, error) {
	cfg := defaultRequestConfig()
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&cfg); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func (c *Client) newHTTPRequest(ctx context.Context, method, target string, cfg requestConfig) (*http.Request, error) {
	u, err := c.buildURL(target, cfg.query)
	if err != nil {
		return nil, err
	}
	var body io.ReadCloser
	if cfg.body.set {
		body, err = cfg.body.factory()
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		if body != nil {
			_ = body.Close()
		}
		return nil, err
	}
	if cfg.body.set && cfg.body.length >= 0 {
		req.ContentLength = cfg.body.length
	}
	appendHeader(req.Header, c.cfg.headers)
	appendHeader(req.Header, cfg.headers)
	if cfg.body.set {
		if contentType := trimContentType(cfg.body.contentType); contentType != "" && req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", contentType)
		}
	}
	return req, nil
}

func (c *Client) doOnce(ctx context.Context, req *http.Request, maxBodyBytes int64, hooks []Hook) (*Response, error) {
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
	body, readErr := readResponseBody(raw.Body, maxBodyBytes)
	resp := newResponse(req, raw, body)
	if readErr != nil {
		readErr = &RequestError{Method: req.Method, URL: req.URL.String(), Err: readErr}
		return resp, runAfterHooks(ctx, hooks, resp, readErr)
	}
	if err := runAfterHooks(ctx, hooks, resp, nil); err != nil {
		return resp, err
	}
	return resp, nil
}

func runAfterHooks(ctx context.Context, hooks []Hook, resp *Response, err error) error {
	var hookErr error
	for _, hook := range hooks {
		hookErr = errors.Join(hookErr, hook.AfterResponse(ctx, resp, err))
	}
	if hookErr != nil {
		if err != nil {
			return errors.Join(err, hookErr)
		}
		return hookErr
	}
	return err
}

func (c *Client) buildURL(target string, reqQuery url.Values) (*url.URL, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	if c.cfg.baseURL != nil && !u.IsAbs() {
		base := *c.cfg.baseURL
		base.Path = joinURLPath(base.Path, u.Path)
		base.RawPath = ""
		base.RawQuery = mergeQuery(base.RawQuery, u.RawQuery)
		base.Fragment = u.Fragment
		u = &base
	}
	query := u.Query()
	appendValues(query, c.cfg.query)
	appendValues(query, reqQuery)
	u.RawQuery = query.Encode()
	return u, nil
}

func joinURLPath(basePath, targetPath string) string {
	if basePath == "" {
		if targetPath == "" {
			return ""
		}
		if strings.HasPrefix(targetPath, "/") {
			return path.Clean(targetPath)
		}
		return "/" + path.Clean(targetPath)
	}
	if targetPath == "" {
		return basePath
	}
	joined := path.Join(strings.TrimRight(basePath, "/"), strings.TrimLeft(targetPath, "/"))
	if strings.HasPrefix(basePath, "/") {
		return joined
	}
	return "/" + joined
}

func mergeQuery(left, right string) string {
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	return fmt.Sprintf("%s&%s", left, right)
}

func readResponseBody(body io.ReadCloser, limit int64) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	defer body.Close()
	if limit <= 0 {
		return io.ReadAll(body)
	}
	limited := io.LimitReader(body, limit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return data, err
	}
	if int64(len(data)) > limit {
		return data[:limit], ErrBodyTooLarge
	}
	return data, nil
}
