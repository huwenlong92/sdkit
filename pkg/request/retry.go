package request

import (
	"context"
	"errors"
	"net/http"
	"time"
)

type RetryConfig struct {
	MaxAttempts int
	WaitMin     time.Duration
	WaitMax     time.Duration
	Methods     []string
	StatusCodes []int
	RetryErrors bool
	ShouldRetry func(ctx context.Context, attempt int, resp *Response, err error) bool
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		WaitMin:     100 * time.Millisecond,
		WaitMax:     time.Second,
		Methods:     []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete},
		StatusCodes: []int{http.StatusRequestTimeout, 425, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout},
		RetryErrors: true,
	}
}

func normalizeRetry(cfg RetryConfig) RetryConfig {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}
	if cfg.WaitMin < 0 {
		cfg.WaitMin = 0
	}
	if cfg.WaitMax <= 0 || cfg.WaitMax < cfg.WaitMin {
		cfg.WaitMax = cfg.WaitMin
	}
	return cfg
}

func shouldRetry(ctx context.Context, cfg RetryConfig, method string, attempt int, resp *Response, err error) bool {
	if attempt >= cfg.MaxAttempts || ctx.Err() != nil {
		return false
	}
	if cfg.ShouldRetry != nil {
		return cfg.ShouldRetry(ctx, attempt, resp, err)
	}
	if !methodAllowed(cfg.Methods, method) {
		return false
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrBodyTooLarge) || errors.Is(err, ErrBodyNotReplayable) {
			return false
		}
		return cfg.RetryErrors
	}
	if resp == nil {
		return false
	}
	return statusAllowed(cfg.StatusCodes, resp.StatusCode)
}

func methodAllowed(methods []string, method string) bool {
	if len(methods) == 0 {
		return false
	}
	for _, item := range methods {
		if item == method {
			return true
		}
	}
	return false
}

func statusAllowed(codes []int, code int) bool {
	for _, item := range codes {
		if item == code {
			return true
		}
	}
	return false
}

func retryWait(cfg RetryConfig, attempt int) time.Duration {
	if cfg.WaitMin <= 0 {
		return 0
	}
	wait := cfg.WaitMin
	for i := 1; i < attempt; i++ {
		wait *= 2
		if cfg.WaitMax > 0 && wait >= cfg.WaitMax {
			return cfg.WaitMax
		}
	}
	return wait
}

func waitRetry(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
