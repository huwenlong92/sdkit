package sdingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var (
	ErrNilContext             = errors.New("sdingest: nil context")
	ErrInvalidConfig          = errors.New("sdingest: invalid client config")
	ErrIdempotencyKeyRequired = errors.New("sdingest: idempotency key is required")
	ErrInvalidCallback        = errors.New("sdingest: invalid callback")
	ErrCallbackSignature      = errors.New("sdingest: callback signature mismatch")
	ErrCallbackTimestamp      = errors.New("sdingest: callback timestamp is outside the allowed window")
)

type APIError struct {
	StatusCode int
	Code       int
	SubCode    string
	Message    string
	Data       json.RawMessage
	RequestID  string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.SubCode != "" {
		return fmt.Sprintf("sdingest: request failed: code=%d sub_code=%s message=%s", e.Code, e.SubCode, e.Message)
	}
	return fmt.Sprintf("sdingest: request failed: code=%d message=%s", e.Code, e.Message)
}

func (e *APIError) Retryable() bool {
	if e == nil {
		return false
	}
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= http.StatusInternalServerError || e.Code == 3001
}

func IsSubCode(err error, subCode string) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.SubCode == subCode
}

type ProtocolError struct {
	StatusCode int
	RequestID  string
	Err        error
}

func (e *ProtocolError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("sdingest: invalid API response: status=%d: %v", e.StatusCode, e.Err)
}

func (e *ProtocolError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
