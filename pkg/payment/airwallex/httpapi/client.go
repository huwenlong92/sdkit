//go:build sdkit_payment_airwallex

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/request"
)

func NewClient(cfg Config) (*Client, error) {
	base := ""
	switch cfg.Environment {
	case Sandbox:
		base = SandboxBaseURL
	case Production:
		base = ProductionBaseURL
	default:
		return nil, fmt.Errorf("%w: explicit airwallex environment required", payment.ErrInvalidRequest)
	}
	if strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.AccountID) == "" {
		return nil, fmt.Errorf("%w: airwallex client ID, API key and account ID required", payment.ErrInvalidRequest)
	}
	if cfg.AuthenticationMode == "" {
		cfg.AuthenticationMode = AuthenticateAccount
	}
	if cfg.AuthenticationMode != AuthenticateAccount && cfg.AuthenticationMode != AuthenticateDefault {
		return nil, fmt.Errorf("%w: unsupported airwallex authentication mode", payment.ErrInvalidRequest)
	}
	if cfg.ReturnURL != "" {
		if err := validateReturnURL(cfg.ReturnURL); err != nil {
			return nil, err
		}
	}
	if cfg.NotifyURL != "" {
		u, err := url.Parse(cfg.NotifyURL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return nil, fmt.Errorf("%w: airwallex webhook URL must be HTTPS", payment.ErrInvalidRequest)
		}
	}
	if cfg.WebhookTolerance < 0 {
		return nil, payment.ErrInvalidRequest
	}
	if cfg.WebhookTolerance == 0 {
		cfg.WebhookTolerance = 5 * time.Minute
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	h := http.Client{Timeout: 30 * time.Second}
	if cfg.HTTPClient != nil {
		h = *cfg.HTTPClient
		if h.Timeout == 0 {
			h.Timeout = 30 * time.Second
		}
	}
	// Never forward authentication headers or bearer tokens through redirects.
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	transport, err := request.NewClient(request.WithHTTPClient(&h), request.WithBaseURL(base), request.WithMaxBodyBytes(1<<20), request.WithStatusValidator(func(int) bool { return true }))
	if err != nil {
		return nil, err
	}
	currencies := payment.DefaultCurrencyMetadata()
	for code, meta := range cfg.CurrencyMetadata {
		if code == "" || code != strings.ToUpper(strings.TrimSpace(code)) || len(code) != 3 || meta.MinorExp < 0 || meta.MinorExp > 6 || (meta.Code != "" && meta.Code != code) {
			return nil, payment.ErrInvalidCurrency
		}
		meta.Code = code
		currencies[code] = meta
	}
	return &Client{
		authenticationMode: cfg.AuthenticationMode,
		environment:        cfg.Environment,
		clientID:           cfg.ClientID,
		apiKey:             cfg.APIKey,
		accountID:          cfg.AccountID,
		webhookSecret:      cfg.WebhookSecret,
		returnURL:          cfg.ReturnURL,
		notifyURL:          cfg.NotifyURL,
		transport:          transport,
		clock:              cfg.Clock,
		tolerance:          cfg.WebhookTolerance,
		currencies:         currencies,
	}, nil
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	for {
		if ctx == nil {
			return "", payment.ErrNilContext
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		c.mu.Lock()
		if c.token != "" && c.clock().Add(30*time.Second).Before(c.expiresAt) {
			token := c.token
			c.mu.Unlock()
			return token, nil
		}
		if wait := c.refreshing; wait != nil {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-wait:
				continue
			}
		}
		c.refreshing = make(chan struct{})
		c.mu.Unlock()
		var result struct {
			Token     string `json:"token"`
			ExpiresAt string `json:"expires_at"`
		}
		opts := []request.RequestOption{request.WithHeader("x-client-id", c.clientID), request.WithHeader("x-api-key", c.apiKey)}
		if c.authenticationMode == AuthenticateAccount {
			opts = append(opts, request.WithHeader("x-login-as", c.accountID))
		}
		err := c.send(ctx, http.MethodPost, "/api/v1/authentication/login", struct{}{}, &result, opts...)
		var expiresAt time.Time
		if err == nil {
			var parseErr error
			expiresAt, parseErr = time.Parse(time.RFC3339Nano, result.ExpiresAt)
			if parseErr != nil {
				// Airwallex production also uses basic ISO-8601 offsets (+0000).
				expiresAt, parseErr = time.Parse("2006-01-02T15:04:05.999999999-0700", result.ExpiresAt)
			}
			if parseErr != nil {
				err = fmt.Errorf("%w: invalid airwallex token expiry", payment.ErrInvalidRequest)
			}
		}
		if err == nil && (result.Token == "" || !expiresAt.After(c.clock().Add(30*time.Second))) {
			err = fmt.Errorf("%w: invalid airwallex authentication response", payment.ErrInvalidRequest)
		}
		c.mu.Lock()
		if err == nil {
			c.token = result.Token
			c.expiresAt = expiresAt
		}
		close(c.refreshing)
		c.refreshing = nil
		c.mu.Unlock()
		return result.Token, err
	}
}

func (c *Client) call(ctx context.Context, method, path string, payload, result any) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	err = c.send(ctx, method, path, payload, result, request.WithBearerToken(token))
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
		// Do not replay mutations automatically. The caller retains the request ID.
		c.mu.Lock()
		if c.token == token {
			c.token = ""
			c.expiresAt = time.Time{}
		}
		c.mu.Unlock()
	}
	return err
}

func (c *Client) send(ctx context.Context, method, path string, payload, result any, opts ...request.RequestOption) error {
	if ctx == nil {
		return payment.ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if payload != nil {
		opts = append(opts, request.WithJSON(payload))
	}
	// Transfers uses the renamed resource and fields introduced in this version.
	if strings.HasPrefix(path, "/api/v1/transfers/") {
		opts = append(opts, request.WithHeader("x-api-version", "2024-09-27"))
	}
	response, err := c.transport.Do(ctx, method, path, opts...)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}
		return fmt.Errorf("airwallex transport failed; outcome may be unknown")
	}
	if response == nil {
		return fmt.Errorf("airwallex empty HTTP response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(response.Body, &failure)
		code := failure.Code
		if len(code) > 80 || !safeCode(code) {
			code = ""
		}
		return &APIError{StatusCode: response.StatusCode, Code: code}
	}
	if result != nil && json.Unmarshal(response.Body, result) != nil {
		return fmt.Errorf("airwallex invalid JSON response")
	}
	return nil
}

func safeCode(code string) bool {
	for _, r := range code {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
