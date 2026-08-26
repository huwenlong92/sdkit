package sdingest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/huwenlong92/sdkit/pkg/request"
)

const (
	defaultMaxResponseBytes = int64(8 << 20)
	defaultTokenSkew        = 30 * time.Second
)

type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

type Config struct {
	BaseURL          string
	AppID            string
	AppSecret        string
	HTTPClient       HTTPDoer
	TokenSkew        time.Duration
	MaxResponseBytes int64
	UserAgent        string
	Clock            func() time.Time
}

type Client struct {
	appID     string
	appSecret string
	transport *request.Client
	tokenSkew time.Duration
	clock     func() time.Time

	tokenMu sync.Mutex
	token   Token
}

type envelope struct {
	ErrCode int             `json:"err_code"`
	SubCode string          `json:"sub_code,omitempty"`
	Message string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
}

func NewClient(config Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	appID := strings.TrimSpace(config.AppID)
	appSecret := strings.TrimSpace(config.AppSecret)
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("%w: base URL must be an absolute HTTP URL without credentials, query, or fragment", ErrInvalidConfig)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: unsupported base URL scheme", ErrInvalidConfig)
	}
	if appID == "" || appSecret == "" {
		return nil, fmt.Errorf("%w: app ID and app secret are required", ErrInvalidConfig)
	}
	tokenSkew := config.TokenSkew
	if tokenSkew <= 0 {
		tokenSkew = defaultTokenSkew
	}
	maxResponseBytes := config.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = defaultMaxResponseBytes
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	userAgent := strings.TrimSpace(config.UserAgent)
	if userAgent == "" {
		userAgent = "sdkit-sdingest-go/1.0"
	}
	httpClient := sharedHTTPClient(config.HTTPClient)
	transport, err := request.NewClient(
		request.WithBaseURL(baseURL),
		request.WithHTTPClient(httpClient),
		request.WithDefaultHeader("Accept", "application/json"),
		request.WithDefaultHeader("User-Agent", userAgent),
		request.WithMaxBodyBytes(maxResponseBytes),
		request.WithRetry(request.RetryConfig{MaxAttempts: 1}),
		request.WithStatusValidator(func(int) bool { return true }),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: configure HTTP transport: %v", ErrInvalidConfig, err)
	}
	return &Client{
		appID: appID, appSecret: appSecret, transport: transport, tokenSkew: tokenSkew, clock: clock,
	}, nil
}

func (c *Client) Authenticate(ctx context.Context) (Token, error) {
	if ctx == nil {
		return Token{}, ErrNilContext
	}
	if c == nil {
		return Token{}, ErrInvalidConfig
	}
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token.AccessToken != "" && c.clock().Before(c.token.expiresAt.Add(-c.tokenSkew)) {
		return c.token, nil
	}
	token, err := c.requestToken(ctx)
	if err != nil {
		return Token{}, err
	}
	c.token = token
	return token, nil
}

func (c *Client) requestToken(ctx context.Context) (Token, error) {
	response, err := c.transport.Post(ctx, "/v1/auth/token", request.WithBasicAuth(c.appID, c.appSecret))
	if err != nil {
		return Token{}, transportError(response, err)
	}
	var token Token
	if err := c.decode(response, &token); err != nil {
		return Token{}, err
	}
	if token.AccessToken == "" {
		return Token{}, &ProtocolError{StatusCode: response.StatusCode, RequestID: requestID(response.Header), Err: errors.New("access token is empty")}
	}
	if token.ExpiresIn <= 0 {
		return Token{}, &ProtocolError{StatusCode: response.StatusCode, RequestID: requestID(response.Header), Err: errors.New("access token expiry is invalid")}
	}
	token.expiresAt = c.clock().Add(time.Duration(token.ExpiresIn) * time.Second)
	return token, nil
}

func (c *Client) do(ctx context.Context, method string, path string, query url.Values, body any, idempotencyKey string, output any) error {
	if ctx == nil {
		return ErrNilContext
	}
	if c == nil {
		return ErrInvalidConfig
	}
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		token, err := c.Authenticate(ctx)
		if err != nil {
			return err
		}
		err = c.doAuthenticated(ctx, method, path, query, body, idempotencyKey, token.AccessToken, output)
		apiErr := new(APIError)
		if attempt == 0 && errors.As(err, &apiErr) && (apiErr.Code == http.StatusUnauthorized || apiErr.StatusCode == http.StatusUnauthorized) {
			c.invalidateToken(token.AccessToken)
			continue
		}
		return err
	}
	return err
}

func (c *Client) doAuthenticated(ctx context.Context, method string, path string, query url.Values, body any, idempotencyKey string, accessToken string, output any) error {
	opts := []request.RequestOption{request.WithBearerToken(accessToken)}
	if len(query) > 0 {
		opts = append(opts, request.WithQueryValues(query))
	}
	if body != nil {
		opts = append(opts, request.WithJSON(body))
	}
	if idempotencyKey != "" {
		opts = append(opts, request.WithHeader("Idempotency-Key", idempotencyKey))
	}
	response, err := c.transport.Do(ctx, method, path, opts...)
	if err != nil {
		return transportError(response, err)
	}
	return c.decode(response, output)
}

func (c *Client) decode(response *request.Response, output any) error {
	if response == nil {
		return &ProtocolError{Err: errors.New("response is unavailable")}
	}
	requestID := requestID(response.Header)
	raw := response.Bytes()
	if len(bytes.TrimSpace(raw)) == 0 {
		return &ProtocolError{StatusCode: response.StatusCode, RequestID: requestID, Err: errors.New("response body is empty")}
	}
	var result envelope
	if err := json.Unmarshal(raw, &result); err != nil {
		return &ProtocolError{StatusCode: response.StatusCode, RequestID: requestID, Err: err}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices || result.ErrCode != http.StatusOK {
		return &APIError{
			StatusCode: response.StatusCode, Code: result.ErrCode, SubCode: result.SubCode,
			Message: result.Message, Data: cloneRaw(result.Data), RequestID: requestID,
			RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), c.clock()),
		}
	}
	if output == nil {
		return nil
	}
	if len(result.Data) == 0 || bytes.Equal(bytes.TrimSpace(result.Data), []byte("null")) {
		return &ProtocolError{StatusCode: response.StatusCode, RequestID: requestID, Err: errors.New("response data is empty")}
	}
	if err := json.Unmarshal(result.Data, output); err != nil {
		return &ProtocolError{StatusCode: response.StatusCode, RequestID: requestID, Err: err}
	}
	return nil
}

func sharedHTTPClient(doer HTTPDoer) *http.Client {
	if doer == nil {
		return http.DefaultClient
	}
	if client, ok := doer.(*http.Client); ok {
		return client
	}
	return &http.Client{Transport: httpDoerTransport{doer: doer}}
}

type httpDoerTransport struct {
	doer HTTPDoer
}

func (t httpDoerTransport) RoundTrip(httpRequest *http.Request) (*http.Response, error) {
	return t.doer.Do(httpRequest)
}

func transportError(response *request.Response, err error) error {
	if errors.Is(err, request.ErrBodyTooLarge) {
		protocolErr := &ProtocolError{Err: errors.New("response body exceeds configured limit")}
		if response != nil {
			protocolErr.StatusCode = response.StatusCode
			protocolErr.RequestID = requestID(response.Header)
		}
		return protocolErr
	}
	return err
}

func (c *Client) invalidateToken(accessToken string) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token.AccessToken == accessToken {
		c.token = Token{}
	}
}

func requestID(header http.Header) string {
	if value := strings.TrimSpace(header.Get("X-Request-ID")); value != "" {
		return value
	}
	return strings.TrimSpace(header.Get("X-Request-Id"))
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		delay := retryAt.Sub(now)
		if delay > 0 {
			return delay
		}
	}
	return 0
}

func cloneRaw(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}
