package sdingest_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/pkg/sdingest"
)

func TestClientCachesTokenAcrossConcurrentRequests(t *testing.T) {
	var tokenRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/auth/token":
			tokenRequests.Add(1)
			if appID, appSecret, ok := request.BasicAuth(); !ok || appID != "app-id" || appSecret != "app-secret" {
				t.Errorf("unexpected basic auth")
			}
			time.Sleep(20 * time.Millisecond)
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{
				"access_token": "cached-token", "token_type": "Bearer", "expires_in": 3600, "scope": []string{"job:read"},
			})
		case "/v1/app/profile":
			if request.Header.Get("Authorization") != "Bearer cached-token" {
				t.Errorf("unexpected authorization header")
			}
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{"app_id": "app-id", "name": "test"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := newClient(t, server.URL)
	var wait sync.WaitGroup
	for range 20 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			profile, err := client.GetAppProfile(context.Background())
			if err != nil {
				t.Errorf("GetAppProfile: %v", err)
				return
			}
			if profile.AppID != "app-id" {
				t.Errorf("app_id = %q", profile.AppID)
			}
		}()
	}
	wait.Wait()
	if count := tokenRequests.Load(); count != 1 {
		t.Fatalf("token requests = %d, want 1", count)
	}
}

func TestClientRefreshesTokenOnceAfterUnauthorized(t *testing.T) {
	var tokenRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/auth/token":
			count := tokenRequests.Add(1)
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{
				"access_token": fmt.Sprintf("token-%d", count), "token_type": "Bearer", "expires_in": 3600,
			})
		case "/v1/app/profile":
			if request.Header.Get("Authorization") == "Bearer token-1" {
				writeEnvelope(t, writer, http.StatusOK, 401, "TOKEN_EXPIRED", nil)
				return
			}
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{"app_id": "app-id"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	profile, err := newClient(t, server.URL).GetAppProfile(context.Background())
	if err != nil {
		t.Fatalf("GetAppProfile: %v", err)
	}
	if profile.AppID != "app-id" || tokenRequests.Load() != 2 {
		t.Fatalf("profile=%+v token_requests=%d", profile, tokenRequests.Load())
	}
}

func TestClientStopsAfterSecondUnauthorized(t *testing.T) {
	var tokenRequests atomic.Int64
	var profileRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/auth/token":
			count := tokenRequests.Add(1)
			writeEnvelope(t, writer, http.StatusOK, http.StatusOK, "", map[string]any{
				"access_token": fmt.Sprintf("token-%d", count), "token_type": "Bearer", "expires_in": 3600,
			})
		case "/v1/app/profile":
			profileRequests.Add(1)
			writeEnvelope(t, writer, http.StatusUnauthorized, http.StatusUnauthorized, "TOKEN_EXPIRED", nil)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	_, err := newClient(t, server.URL).GetAppProfile(context.Background())
	var apiErr *sdingest.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("error = %T %v", err, err)
	}
	if tokenRequests.Load() != 2 || profileRequests.Load() != 2 {
		t.Fatalf("token requests = %d, profile requests = %d", tokenRequests.Load(), profileRequests.Load())
	}
}

func TestClientPreservesCustomHTTPDoerCompatibility(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/auth/token":
			writeEnvelope(t, writer, http.StatusOK, http.StatusOK, "", map[string]any{"access_token": "token", "expires_in": 3600})
		case "/v1/app/profile":
			writeEnvelope(t, writer, http.StatusOK, http.StatusOK, "", map[string]any{"app_id": "app-id"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client, err := sdingest.NewClient(sdingest.Config{
		BaseURL: server.URL, AppID: "app-id", AppSecret: "app-secret",
		HTTPClient: httpDoerFunc(func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return http.DefaultClient.Do(request)
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := client.GetAppProfile(context.Background())
	if err != nil || profile.AppID != "app-id" {
		t.Fatalf("GetAppProfile() = %+v, %v", profile, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("HTTPDoer calls = %d, want 2", calls.Load())
	}
}

func TestCreateJobRequiresAndForwardsStableIdempotencyKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/auth/token":
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{"access_token": "token", "expires_in": 3600})
		case "/v1/ingest/job/create":
			if value := request.Header.Get("Idempotency-Key"); value != "import-receipt-42" {
				t.Errorf("idempotency key = %q", value)
			}
			var input sdingest.CreateJobInput
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Errorf("decode request: %v", err)
			}
			if input.ExternalRef != "asset-42" || input.Source.Mode != "share_link" {
				t.Errorf("input = %+v", input)
			}
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{"job_id": "job-42", "status": "pending"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client := newClient(t, server.URL)

	_, err := client.CreateJob(context.Background(), sdingest.CreateJobInput{}, "")
	if !errors.Is(err, sdingest.ErrIdempotencyKeyRequired) {
		t.Fatalf("missing key error = %v", err)
	}
	job, err := client.CreateJob(context.Background(), sdingest.CreateJobInput{
		ExternalRef: "asset-42",
		Source:      sdingest.Source{Mode: "share_link", URL: "https://pan.baidu.com/s/example", Password: "test"},
	}, "import-receipt-42")
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if job.JobID != "job-42" {
		t.Fatalf("job_id = %q", job.JobID)
	}
}

func TestClientSupportsAutoAndManualManifestModes(t *testing.T) {
	var createInputs []sdingest.CreateJobInput
	var confirmInputs []sdingest.ConfirmJobManifestInput
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/auth/token":
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{"access_token": "token", "expires_in": 3600})
		case "/v1/ingest/job/create":
			var input sdingest.CreateJobInput
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Errorf("decode create request: %v", err)
			}
			createInputs = append(createInputs, input)
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{
				"job_id": "job-manual", "manifest_mode": input.ManifestMode, "status": "pending",
			})
		case "/v1/ingest/job/manifest-confirm":
			var input sdingest.ConfirmJobManifestInput
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Errorf("decode confirm request: %v", err)
			}
			confirmInputs = append(confirmInputs, input)
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{
				"job_id": input.JobID, "manifest_mode": sdingest.ManifestModeManual, "status": "pending", "phase": "download",
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := newClient(t, server.URL)
	manual, err := client.CreateJob(context.Background(), sdingest.CreateJobInput{
		ManifestMode: sdingest.ManifestModeManual,
		Source:       sdingest.Source{Mode: "share_link", URL: "https://pan.baidu.com/s/example"},
	}, "job-manual-create")
	if err != nil {
		t.Fatalf("CreateJob(manual): %v", err)
	}
	if manual.ManifestMode != sdingest.ManifestModeManual {
		t.Fatalf("manual job mode = %q", manual.ManifestMode)
	}
	if _, err := client.CreateJob(context.Background(), sdingest.CreateJobInput{
		Source: sdingest.Source{Mode: "share_link", URL: "https://pan.baidu.com/s/example"},
	}, "job-auto-create"); err != nil {
		t.Fatalf("CreateJob(auto default): %v", err)
	}

	confirmed, err := client.ConfirmJobManifestSelection(context.Background(), sdingest.ConfirmJobManifestInput{
		JobID: "job-manual", Revision: 3, Hash: strings.Repeat("a", 64), ItemIDs: []string{"itm-1", "itm-2"},
	}, "job-manual-confirm")
	if err != nil {
		t.Fatalf("ConfirmJobManifestSelection: %v", err)
	}
	if confirmed.JobID != "job-manual" || len(confirmInputs) != 1 || !slices.Equal(confirmInputs[0].ItemIDs, []string{"itm-1", "itm-2"}) {
		t.Fatalf("confirmed=%+v inputs=%+v", confirmed, confirmInputs)
	}
	if len(createInputs) != 2 || createInputs[0].ManifestMode != sdingest.ManifestModeManual || createInputs[1].ManifestMode != "" {
		t.Fatalf("create inputs = %+v", createInputs)
	}
}

func TestClientReturnsMachineReadableAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/auth/token" {
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{"access_token": "token", "expires_in": 3600})
			return
		}
		writer.Header().Set("Retry-After", "17")
		writer.Header().Set("X-Request-ID", "request-429")
		writeEnvelope(t, writer, http.StatusTooManyRequests, 3001, "ACCOUNT_CAPACITY_EXHAUSTED", nil)
	}))
	defer server.Close()

	_, err := newClient(t, server.URL).GetJob(context.Background(), "job-1")
	var apiErr *sdingest.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v", err, err)
	}
	if apiErr.SubCode != "ACCOUNT_CAPACITY_EXHAUSTED" || apiErr.RequestID != "request-429" || apiErr.RetryAfter != 17*time.Second || !apiErr.Retryable() {
		t.Fatalf("api error = %+v", apiErr)
	}
	if !sdingest.IsSubCode(err, "ACCOUNT_CAPACITY_EXHAUSTED") {
		t.Fatal("IsSubCode returned false")
	}
}

func TestClientPropagatesContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/auth/token" {
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{"access_token": "token", "expires_in": 3600})
			return
		}
		<-request.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := newClient(t, server.URL).GetJob(ctx, "job-1")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
}

func TestVerifyAndDecodeCallback(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	body := []byte(`{"event_id":"evt-1","event":"job.succeeded","timestamp":"2026-08-25T06:00:00Z","data":{"job_id":"job-1","status":"succeeded","phase":"cleanup","revision":9}}`)
	header := make(http.Header)
	header.Set(sdingest.CallbackHeaderEventID, "evt-1")
	header.Set(sdingest.CallbackHeaderTimestamp, strconv.FormatInt(now.Unix(), 10))
	header.Set(sdingest.CallbackHeaderSignature, callbackSignature("callback-secret", now.Unix(), body))

	event, err := sdingest.VerifyAndDecodeCallback(header, body, "callback-secret", now, time.Minute)
	if err != nil {
		t.Fatalf("VerifyAndDecodeCallback: %v", err)
	}
	if event.Event != sdingest.CallbackEventSucceeded || event.Data.JobID != "job-1" || event.Data.Revision != 9 {
		t.Fatalf("event = %+v", event)
	}
	header.Set(sdingest.CallbackHeaderSignature, "v1=bad")
	_, err = sdingest.VerifyAndDecodeCallback(header, body, "callback-secret", now, time.Minute)
	if !errors.Is(err, sdingest.ErrCallbackSignature) {
		t.Fatalf("signature error = %v", err)
	}
	header.Set(sdingest.CallbackHeaderSignature, callbackSignature("callback-secret", now.Unix(), body))
	_, err = sdingest.VerifyAndDecodeCallback(header, body, "callback-secret", now.Add(10*time.Minute), time.Minute)
	if !errors.Is(err, sdingest.ErrCallbackTimestamp) {
		t.Fatalf("timestamp error = %v", err)
	}
}

func TestCallbackManagementUsesConfiguredBasePathAndStableKeys(t *testing.T) {
	paths := make([]string, 0, 9)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/auth/token" {
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{"access_token": "token", "expires_in": 3600})
			return
		}
		paths = append(paths, request.URL.Path)
		switch request.URL.Path {
		case "/v1/callback/create":
			if request.Header.Get("Idempotency-Key") != "callback-create-1" {
				t.Errorf("create idempotency key = %q", request.Header.Get("Idempotency-Key"))
			}
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{
				"callback_id": "cb-1", "name": "DreamIP", "url": "https://example.com/callback",
				"events": []string{sdingest.CallbackEventFailed}, "status": "enabled", "secret_ver": 1,
				"signing_secret": "callback-secret-1",
			})
		case "/v1/callback/detail":
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{"callback_id": "cb-1", "status": "enabled"})
		case "/v1/callback/list":
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{"list": []map[string]any{{"callback_id": "cb-1"}}, "total": 1})
		case "/v1/callback/log-list":
			if request.URL.Query().Get("callback_id") != "cb-1" || request.URL.Query().Get("job_id") != "job-1" {
				t.Errorf("callback log query = %s", request.URL.RawQuery)
			}
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{
				"list":  []map[string]any{{"event_id": "evt-1", "callback_id": "cb-1", "job_id": "job-1", "status": "succeeded"}},
				"total": 1,
			})
		case "/v1/callback/secret-rotate":
			if request.Header.Get("Idempotency-Key") != "callback-rotate-1" {
				t.Errorf("rotate idempotency key = %q", request.Header.Get("Idempotency-Key"))
			}
			writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{
				"callback_id": "cb-1", "status": "enabled", "secret_ver": 2, "signing_secret": "callback-secret-2",
			})
		case "/v1/callback/disable", "/v1/callback/enable":
			if request.Header.Get("Idempotency-Key") == "" {
				t.Error("callback state change omitted idempotency key")
			}
			writeEnvelope(t, writer, http.StatusOK, 200, "", nil)
		case "/v1/callback/replay":
			if request.Header.Get("Idempotency-Key") != "callback-replay-1" {
				t.Errorf("replay idempotency key = %q", request.Header.Get("Idempotency-Key"))
			}
			var input struct {
				EventID string `json:"event_id"`
			}
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil || input.EventID != "evt-1" {
				t.Errorf("replay input = %+v, err = %v", input, err)
			}
			writeEnvelope(t, writer, http.StatusOK, 200, "", nil)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := newClient(t, server.URL)
	created, err := client.CreateCallback(context.Background(), sdingest.CreateCallbackInput{
		Name: "DreamIP", URL: "https://example.com/callback", Events: []string{sdingest.CallbackEventFailed},
	}, "callback-create-1")
	if err != nil || created.CallbackID != "cb-1" || created.SigningSecret == "" {
		t.Fatalf("CreateCallback() = %+v, %v", created, err)
	}
	if detail, err := client.GetCallback(context.Background(), created.CallbackID); err != nil || detail.CallbackID != created.CallbackID {
		t.Fatalf("GetCallback() = %+v, %v", detail, err)
	}
	if page, err := client.ListCallbacks(context.Background(), sdingest.ListCallbacksInput{Page: 1, Limit: 20}); err != nil || page.Total != 1 {
		t.Fatalf("ListCallbacks() = %+v, %v", page, err)
	}
	if page, err := client.ListCallbackLogs(context.Background(), sdingest.ListCallbackLogsInput{
		Page: 1, Limit: 20, CallbackID: created.CallbackID, JobID: "job-1",
	}); err != nil || page.Total != 1 || page.List[0].EventID != "evt-1" {
		t.Fatalf("ListCallbackLogs() = %+v, %v", page, err)
	}
	rotated, err := client.RotateCallbackSecret(context.Background(), created.CallbackID, "callback-rotate-1")
	if err != nil || rotated.SecretVer != 2 || rotated.SigningSecret == created.SigningSecret {
		t.Fatalf("RotateCallbackSecret() = %+v, %v", rotated, err)
	}
	if err := client.DisableCallback(context.Background(), created.CallbackID, "callback-disable-1"); err != nil {
		t.Fatal(err)
	}
	if err := client.EnableCallback(context.Background(), created.CallbackID, "callback-enable-1"); err != nil {
		t.Fatal(err)
	}
	if err := client.ReplayCallback(context.Background(), "evt-1", "callback-replay-1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/v1/callback/create", "/v1/callback/detail", "/v1/callback/list", "/v1/callback/log-list",
		"/v1/callback/secret-rotate", "/v1/callback/disable", "/v1/callback/enable", "/v1/callback/replay",
	}
	if !slices.Equal(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

func TestClientRejectsInvalidProtocolResponsesWithoutEchoingBody(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		maxBytes int64
	}{
		{name: "empty response"},
		{name: "non json response", body: "sensitive-response-content"},
		{name: "oversized response", body: strings.Repeat("x", 300), maxBytes: 256},
		{name: "missing success data", body: `{"err_code":200,"msg":"OK","data":null}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/v1/auth/token" {
					writeEnvelope(t, writer, http.StatusOK, 200, "", map[string]any{"access_token": "token", "expires_in": 3600})
					return
				}
				writer.WriteHeader(http.StatusOK)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()

			maxBytes := test.maxBytes
			if maxBytes == 0 {
				maxBytes = 1024
			}
			client, err := sdingest.NewClient(sdingest.Config{
				BaseURL: server.URL, AppID: "app-id", AppSecret: "app-secret", MaxResponseBytes: maxBytes,
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.GetJob(context.Background(), "job-1")
			var protocolErr *sdingest.ProtocolError
			if !errors.As(err, &protocolErr) {
				t.Fatalf("error = %T %v", err, err)
			}
			if strings.Contains(err.Error(), test.body) && test.body != "" {
				t.Fatalf("protocol error echoed response body: %v", err)
			}
		})
	}
}

func newClient(t *testing.T, baseURL string) *sdingest.Client {
	t.Helper()
	client, err := sdingest.NewClient(sdingest.Config{BaseURL: baseURL, AppID: "app-id", AppSecret: "app-secret"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func writeEnvelope(t *testing.T, writer http.ResponseWriter, status int, code int, subCode string, data any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(map[string]any{"err_code": code, "sub_code": subCode, "msg": http.StatusText(code), "data": data}); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func callbackSignature(secret string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

type httpDoerFunc func(*http.Request) (*http.Response, error)

func (f httpDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}
