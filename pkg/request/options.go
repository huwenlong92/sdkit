package request

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultTimeout      = 30 * time.Second
	DefaultMaxBodyBytes = 10 * 1024 * 1024
)

type StatusValidator func(statusCode int) bool

type ClientOption func(*clientConfig) error

type RequestOption func(*requestConfig) error

type clientConfig struct {
	httpClient      *http.Client
	baseURL         *url.URL
	headers         http.Header
	query           url.Values
	maxBodyBytes    int64
	retry           RetryConfig
	hooks           []Hook
	statusValidator StatusValidator
}

type requestConfig struct {
	query           url.Values
	headers         http.Header
	body            bodyConfig
	maxBodyBytes    *int64
	retry           *RetryConfig
	hooks           []Hook
	statusValidator StatusValidator
	decodeJSON      any
	timeout         time.Duration
}

type bodyConfig struct {
	contentType string
	factory     func() (io.ReadCloser, error)
	length      int64
	set         bool
}

func defaultClientConfig() clientConfig {
	return clientConfig{
		httpClient:      &http.Client{Timeout: DefaultTimeout},
		headers:         make(http.Header),
		query:           make(url.Values),
		maxBodyBytes:    DefaultMaxBodyBytes,
		retry:           RetryConfig{MaxAttempts: 1},
		statusValidator: defaultStatusValidator,
	}
}

func defaultRequestConfig() requestConfig {
	return requestConfig{
		query:   make(url.Values),
		headers: make(http.Header),
	}
}

func defaultStatusValidator(code int) bool {
	return code >= 200 && code <= 299
}

func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *clientConfig) error {
		if client == nil {
			return ErrNilHTTPClient
		}
		c.httpClient = client
		return nil
	}
}

func WithTransport(transport http.RoundTripper) ClientOption {
	return func(c *clientConfig) error {
		if transport == nil {
			return ErrNilTransport
		}
		client := cloneHTTPClient(c.httpClient)
		client.Transport = transport
		c.httpClient = client
		return nil
	}
}

func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *clientConfig) error {
		client := cloneHTTPClient(c.httpClient)
		client.Timeout = timeout
		c.httpClient = client
		return nil
	}
}

func WithBaseURL(rawURL string) ClientOption {
	return func(c *clientConfig) error {
		u, err := url.Parse(rawURL)
		if err != nil {
			return err
		}
		if u.Scheme == "" || u.Host == "" {
			return errors.New("request: base url must be absolute")
		}
		c.baseURL = u
		return nil
	}
}

func WithDefaultHeader(name, value string) ClientOption {
	return func(c *clientConfig) error {
		c.headers.Set(name, value)
		return nil
	}
}

func WithDefaultHeaders(headers http.Header) ClientOption {
	return func(c *clientConfig) error {
		for key, values := range headers {
			c.headers.Del(key)
			for _, value := range values {
				c.headers.Add(key, value)
			}
		}
		return nil
	}
}

func WithDefaultQuery(name, value string) ClientOption {
	return func(c *clientConfig) error {
		c.query.Set(name, value)
		return nil
	}
}

func WithMaxBodyBytes(n int64) ClientOption {
	return func(c *clientConfig) error {
		c.maxBodyBytes = n
		return nil
	}
}

func WithRetry(cfg RetryConfig) ClientOption {
	return func(c *clientConfig) error {
		c.retry = normalizeRetry(cfg)
		return nil
	}
}

func WithHook(hook Hook) ClientOption {
	return func(c *clientConfig) error {
		if hook != nil {
			c.hooks = append(c.hooks, hook)
		}
		return nil
	}
}

func WithBeforeHook(fn func(context.Context, *http.Request) error) ClientOption {
	return func(c *clientConfig) error {
		if fn != nil {
			c.hooks = append(c.hooks, HookFunc{Before: func(ctx context.Context, req *http.Request) error {
				return fn(ctx, req)
			}})
		}
		return nil
	}
}

func WithAfterHook(fn func(context.Context, *Response, error) error) ClientOption {
	return func(c *clientConfig) error {
		if fn != nil {
			c.hooks = append(c.hooks, HookFunc{After: func(ctx context.Context, resp *Response, err error) error {
				return fn(ctx, resp, err)
			}})
		}
		return nil
	}
}

func WithStatusValidator(fn StatusValidator) ClientOption {
	return func(c *clientConfig) error {
		if fn == nil {
			c.statusValidator = defaultStatusValidator
			return nil
		}
		c.statusValidator = fn
		return nil
	}
}

func WithProxy(proxyURL string) ClientOption {
	return func(c *clientConfig) error {
		u, err := url.Parse(proxyURL)
		if err != nil {
			return err
		}
		transport := cloneDefaultTransport()
		transport.Proxy = http.ProxyURL(u)
		client := cloneHTTPClient(c.httpClient)
		client.Transport = transport
		c.httpClient = client
		return nil
	}
}

func WithTLSConfig(tlsConfig *tls.Config) ClientOption {
	return func(c *clientConfig) error {
		transport := cloneDefaultTransport()
		if tlsConfig != nil {
			transport.TLSClientConfig = tlsConfig.Clone()
		}
		client := cloneHTTPClient(c.httpClient)
		client.Transport = transport
		c.httpClient = client
		return nil
	}
}

func WithQuery(name, value string) RequestOption {
	return func(c *requestConfig) error {
		c.query.Set(name, value)
		return nil
	}
}

func WithQueryValues(values url.Values) RequestOption {
	return func(c *requestConfig) error {
		for key, items := range values {
			c.query.Del(key)
			for _, item := range items {
				c.query.Add(key, item)
			}
		}
		return nil
	}
}

func WithHeader(name, value string) RequestOption {
	return func(c *requestConfig) error {
		c.headers.Set(name, value)
		return nil
	}
}

func WithHeaders(headers http.Header) RequestOption {
	return func(c *requestConfig) error {
		for key, values := range headers {
			c.headers.Del(key)
			for _, value := range values {
				c.headers.Add(key, value)
			}
		}
		return nil
	}
}

func WithBearerToken(token string) RequestOption {
	return WithHeader("Authorization", "Bearer "+token)
}

func WithBasicAuth(username, password string) RequestOption {
	return func(c *requestConfig) error {
		req := &http.Request{Header: make(http.Header)}
		req.SetBasicAuth(username, password)
		c.headers.Set("Authorization", req.Header.Get("Authorization"))
		return nil
	}
}

func WithJSON(v any) RequestOption {
	return func(c *requestConfig) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return setBytesBody(c, "application/json", b)
	}
}

func WithForm(values url.Values) RequestOption {
	return func(c *requestConfig) error {
		return setBytesBody(c, "application/x-www-form-urlencoded", []byte(values.Encode()))
	}
}

func WithFormMap(values map[string]string) RequestOption {
	form := make(url.Values, len(values))
	for key, value := range values {
		form.Set(key, value)
	}
	return WithForm(form)
}

func WithBody(contentType string, body io.Reader) RequestOption {
	return func(c *requestConfig) error {
		if body == nil {
			return ErrNilBody
		}
		if c.body.set {
			return ErrBodyAlreadySet
		}
		used := false
		c.body = bodyConfig{
			contentType: contentType,
			length:      -1,
			set:         true,
			factory: func() (io.ReadCloser, error) {
				if used {
					return nil, ErrBodyNotReplayable
				}
				used = true
				return io.NopCloser(body), nil
			},
		}
		return nil
	}
}

func WithBytes(contentType string, body []byte) RequestOption {
	return func(c *requestConfig) error {
		return setBytesBody(c, contentType, append([]byte(nil), body...))
	}
}

func WithString(contentType, body string) RequestOption {
	return WithBytes(contentType, []byte(body))
}

func WithMultipart(parts ...MultipartPart) RequestOption {
	return func(c *requestConfig) error {
		b, contentType, err := encodeMultipart(parts)
		if err != nil {
			return err
		}
		return setBytesBody(c, contentType, b)
	}
}

func WithDecodeJSON(v any) RequestOption {
	return func(c *requestConfig) error {
		c.decodeJSON = v
		return nil
	}
}

func WithExpectedStatus(codes ...int) RequestOption {
	allowed := append([]int(nil), codes...)
	return func(c *requestConfig) error {
		c.statusValidator = func(statusCode int) bool {
			return statusAllowed(allowed, statusCode)
		}
		return nil
	}
}

func WithRequestStatusValidator(fn StatusValidator) RequestOption {
	return func(c *requestConfig) error {
		c.statusValidator = fn
		return nil
	}
}

func WithRequestMaxBodyBytes(n int64) RequestOption {
	return func(c *requestConfig) error {
		c.maxBodyBytes = &n
		return nil
	}
}

func WithRequestRetry(cfg RetryConfig) RequestOption {
	return func(c *requestConfig) error {
		normalized := normalizeRetry(cfg)
		c.retry = &normalized
		return nil
	}
}

func WithRequestTimeout(timeout time.Duration) RequestOption {
	return func(c *requestConfig) error {
		c.timeout = timeout
		return nil
	}
}

func WithRequestHook(hook Hook) RequestOption {
	return func(c *requestConfig) error {
		if hook != nil {
			c.hooks = append(c.hooks, hook)
		}
		return nil
	}
}

func setBytesBody(c *requestConfig, contentType string, body []byte) error {
	if c.body.set {
		return ErrBodyAlreadySet
	}
	data := append([]byte(nil), body...)
	c.body = bodyConfig{
		contentType: contentType,
		length:      int64(len(data)),
		set:         true,
		factory: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(data)), nil
		},
	}
	return nil
}

func cloneHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		return &http.Client{}
	}
	copy := *client
	return &copy
}

func cloneDefaultTransport() *http.Transport {
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		return transport.Clone()
	}
	return &http.Transport{}
}

func appendValues(dst, src url.Values) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func appendHeader(dst, src http.Header) {
	for key, values := range src {
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func trimContentType(contentType string) string {
	return strings.TrimSpace(contentType)
}
