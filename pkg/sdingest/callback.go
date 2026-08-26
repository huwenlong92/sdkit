package sdingest

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	CallbackHeaderEventID   = "X-SDIngest-Event-ID"
	CallbackHeaderTimestamp = "X-SDIngest-Timestamp"
	CallbackHeaderSignature = "X-SDIngest-Signature"

	CallbackEventManifestReady    = "job.manifest.ready"
	CallbackEventPhaseChanged     = "job.phase_changed"
	CallbackEventProgressChanged  = "job.progress_changed"
	CallbackEventStalled          = "job.stalled"
	CallbackEventRetrying         = "job.retrying"
	CallbackEventAccountWaiting   = "job.account.waiting"
	CallbackEventAccountRecovered = "job.account.recovered"
	CallbackEventTargetWaiting    = "job.target.waiting"
	CallbackEventTargetRecovered  = "job.target.recovered"
	CallbackEventSucceeded        = "job.succeeded"
	CallbackEventPartial          = "job.partial"
	CallbackEventFailed           = "job.failed"
	CallbackEventCanceled         = "job.canceled"

	defaultCallbackTolerance = 5 * time.Minute
)

type CallbackProgress struct {
	CurrentItemID string    `json:"current_item_id,omitempty"`
	DownBytes     int64     `json:"down_bytes"`
	UpBytes       int64     `json:"up_bytes"`
	TotalBytes    int64     `json:"total_bytes"`
	ItemTotal     int64     `json:"item_total"`
	ItemDone      int64     `json:"item_done"`
	ItemFailed    int64     `json:"item_failed"`
	IsAlive       bool      `json:"is_alive"`
	SnapshotAt    time.Time `json:"snapshot_at"`
}

type CallbackEnvelope struct {
	EventID   string       `json:"event_id"`
	Event     string       `json:"event"`
	Timestamp time.Time    `json:"timestamp"`
	Data      CallbackData `json:"data"`
}

type CallbackData struct {
	JobID         string            `json:"job_id"`
	ExternalRef   string            `json:"external_ref,omitempty"`
	Status        string            `json:"status"`
	Phase         string            `json:"phase"`
	Revision      int64             `json:"revision"`
	WaitingReason string            `json:"waiting_reason,omitempty"`
	ErrorCode     string            `json:"error_code,omitempty"`
	Progress      *CallbackProgress `json:"progress,omitempty"`
}

type Callback struct {
	CallbackID string     `json:"callback_id"`
	AppID      string     `json:"app_id,omitempty"`
	Name       string     `json:"name"`
	URL        string     `json:"url"`
	Events     []string   `json:"events"`
	Status     string     `json:"status"`
	SecretVer  int64      `json:"secret_ver"`
	LastTestAt *time.Time `json:"last_test_at,omitempty"`
	Error      string     `json:"err,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type CallbackCredential struct {
	Callback
	SigningSecret string `json:"signing_secret"`
}

type ListCallbacksInput struct {
	Page   int
	Limit  int
	Status string
	Search string
}

type CreateCallbackInput struct {
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

type CallbackLog struct {
	EventID    string          `json:"event_id"`
	Event      string          `json:"event"`
	CallbackID string          `json:"callback_id"`
	JobID      string          `json:"job_id"`
	AttemptNo  int             `json:"attempt_no"`
	SecretVer  int64           `json:"secret_ver"`
	Status     string          `json:"status"`
	LatencyMS  int64           `json:"latency_ms"`
	NextAt     *time.Time      `json:"next_at,omitempty"`
	Error      string          `json:"err,omitempty"`
	Response   string          `json:"response,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	Callback   json.RawMessage `json:"callback,omitempty"`
	Job        json.RawMessage `json:"job,omitempty"`
}

type ListCallbackLogsInput struct {
	Page       int
	Limit      int
	CallbackID string
	JobID      string
	Status     string
}

func (c *Client) ListCallbacks(ctx context.Context, input ListCallbacksInput) (Page[Callback], error) {
	query := paginationQuery(input.Page, input.Limit)
	setQuery(query, "status", input.Status)
	setQuery(query, "search", input.Search)
	var result Page[Callback]
	err := c.do(ctx, http.MethodGet, "/v1/callback/list", query, nil, "", &result)
	return result, err
}

func (c *Client) GetCallback(ctx context.Context, callbackID string) (Callback, error) {
	query := url.Values{"callback_id": {strings.TrimSpace(callbackID)}}
	var result Callback
	err := c.do(ctx, http.MethodGet, "/v1/callback/detail", query, nil, "", &result)
	return result, err
}

func (c *Client) ListCallbackLogs(ctx context.Context, input ListCallbackLogsInput) (Page[CallbackLog], error) {
	query := paginationQuery(input.Page, input.Limit)
	setQuery(query, "callback_id", input.CallbackID)
	setQuery(query, "job_id", input.JobID)
	setQuery(query, "status", input.Status)
	var result Page[CallbackLog]
	err := c.do(ctx, http.MethodGet, "/v1/callback/log-list", query, nil, "", &result)
	return result, err
}

func (c *Client) CreateCallback(ctx context.Context, input CreateCallbackInput, idempotencyKey string) (CallbackCredential, error) {
	key, err := requiredIdempotencyKey(idempotencyKey)
	if err != nil {
		return CallbackCredential{}, err
	}
	var result CallbackCredential
	err = c.do(ctx, http.MethodPost, "/v1/callback/create", nil, input, key, &result)
	return result, err
}

func (c *Client) RotateCallbackSecret(ctx context.Context, callbackID string, idempotencyKey string) (CallbackCredential, error) {
	key, err := requiredIdempotencyKey(idempotencyKey)
	if err != nil {
		return CallbackCredential{}, err
	}
	input := struct {
		CallbackID string `json:"callback_id"`
	}{CallbackID: strings.TrimSpace(callbackID)}
	var result CallbackCredential
	err = c.do(ctx, http.MethodPost, "/v1/callback/secret-rotate", nil, input, key, &result)
	return result, err
}

func (c *Client) EnableCallback(ctx context.Context, callbackID string, idempotencyKey string) error {
	return c.setCallbackEnabled(ctx, callbackID, idempotencyKey, true)
}

func (c *Client) DisableCallback(ctx context.Context, callbackID string, idempotencyKey string) error {
	return c.setCallbackEnabled(ctx, callbackID, idempotencyKey, false)
}

func (c *Client) ReplayCallback(ctx context.Context, eventID string, idempotencyKey string) error {
	key, err := requiredIdempotencyKey(idempotencyKey)
	if err != nil {
		return err
	}
	input := struct {
		EventID string `json:"event_id"`
	}{EventID: strings.TrimSpace(eventID)}
	return c.do(ctx, http.MethodPost, "/v1/callback/replay", nil, input, key, nil)
}

func (c *Client) setCallbackEnabled(ctx context.Context, callbackID string, idempotencyKey string, enabled bool) error {
	key, err := requiredIdempotencyKey(idempotencyKey)
	if err != nil {
		return err
	}
	path := "/v1/callback/disable"
	if enabled {
		path = "/v1/callback/enable"
	}
	input := struct {
		CallbackID string `json:"callback_id"`
	}{CallbackID: strings.TrimSpace(callbackID)}
	return c.do(ctx, http.MethodPost, path, nil, input, key, nil)
}

func VerifyAndDecodeCallback(header http.Header, body []byte, signingSecret string, now time.Time, tolerance time.Duration) (CallbackEnvelope, error) {
	if header == nil || len(body) == 0 || strings.TrimSpace(signingSecret) == "" {
		return CallbackEnvelope{}, ErrInvalidCallback
	}
	timestamp, err := strconv.ParseInt(strings.TrimSpace(header.Get(CallbackHeaderTimestamp)), 10, 64)
	if err != nil || timestamp <= 0 {
		return CallbackEnvelope{}, ErrInvalidCallback
	}
	if tolerance <= 0 {
		tolerance = defaultCallbackTolerance
	}
	if now.IsZero() {
		now = time.Now()
	}
	difference := now.Sub(time.Unix(timestamp, 0))
	if difference < 0 {
		difference = -difference
	}
	if difference > tolerance {
		return CallbackEnvelope{}, ErrCallbackTimestamp
	}
	expected := callbackSignature(signingSecret, timestamp, body)
	provided := strings.TrimSpace(header.Get(CallbackHeaderSignature))
	if !hmac.Equal([]byte(expected), []byte(provided)) {
		return CallbackEnvelope{}, ErrCallbackSignature
	}
	var result CallbackEnvelope
	if err := json.Unmarshal(body, &result); err != nil {
		return CallbackEnvelope{}, errors.Join(ErrInvalidCallback, err)
	}
	eventID := strings.TrimSpace(header.Get(CallbackHeaderEventID))
	if eventID == "" || result.EventID != eventID || result.Event == "" || result.Data.JobID == "" || result.Data.Revision <= 0 {
		return CallbackEnvelope{}, ErrInvalidCallback
	}
	return result, nil
}

func callbackSignature(signingSecret string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(signingSecret))
	_, _ = mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}
