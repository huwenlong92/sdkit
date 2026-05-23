package request_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/pkg/request"
)

func TestClientBuildsRequestAndDecodesJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users" {
			t.Fatalf("path = %q, want /api/users", r.URL.Path)
		}
		if r.URL.Query().Get("from") != "base" || r.URL.Query().Get("target") != "1" {
			t.Fatalf("query = %q, want base and target query", r.URL.RawQuery)
		}
		if r.URL.Query().Get("client") != "yes" || r.URL.Query().Get("request") != "yes" {
			t.Fatalf("query = %q, want client and request query", r.URL.RawQuery)
		}
		if r.Header.Get("X-App") != "request" {
			t.Fatalf("X-App = %q, want request", r.Header.Get("X-App"))
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("Authorization = %q, want bearer token", r.Header.Get("Authorization"))
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", ct)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["name"] != "sdkit" {
			t.Fatalf("body name = %q, want sdkit", body["name"])
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := request.NewClient(
		request.WithBaseURL(server.URL+"/api?from=base"),
		request.WithDefaultHeader("X-App", "client"),
		request.WithDefaultQuery("client", "yes"),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	var out struct {
		OK bool `json:"ok"`
	}
	resp, err := client.Post(
		context.Background(),
		"/users?target=1",
		request.WithHeader("X-App", "request"),
		request.WithBearerToken("token"),
		request.WithQuery("request", "yes"),
		request.WithJSON(map[string]string{"name": "sdkit"}),
		request.WithDecodeJSON(&out),
	)
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK || !out.OK {
		t.Fatalf("status=%d out=%+v, want 200 ok", resp.StatusCode, out)
	}
}

func TestStatusErrorRetainsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	resp, err := request.Get(context.Background(), server.URL)
	if err == nil {
		t.Fatal("Get() error = nil, want status error")
	}
	var statusErr *request.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error = %T, want *request.StatusError", err)
	}
	if statusErr.StatusCode() != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", statusErr.StatusCode())
	}
	if resp == nil || resp.String() != `{"error":"unauthorized"}` {
		t.Fatalf("response body = %q, want error body", resp.String())
	}
}

func TestRetryGETAndDoesNotRetryPOSTByDefault(t *testing.T) {
	var getCount int32
	var postCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if atomic.AddInt32(&getCount, 1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte("ok"))
		case http.MethodPost:
			atomic.AddInt32(&postCount, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()

	retry := request.DefaultRetryConfig()
	retry.WaitMin = 0
	retry.WaitMax = 0
	client, err := request.NewClient(request.WithRetry(retry))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	resp, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if resp.String() != "ok" || atomic.LoadInt32(&getCount) != 2 {
		t.Fatalf("body=%q getCount=%d, want ok after retry", resp.String(), getCount)
	}

	_, err = client.Post(context.Background(), server.URL, request.WithJSON(map[string]string{"x": "y"}))
	if err == nil {
		t.Fatal("Post() error = nil, want status error")
	}
	if atomic.LoadInt32(&postCount) != 1 {
		t.Fatalf("postCount = %d, want 1", postCount)
	}
}

func TestFormAndMultipartBodies(t *testing.T) {
	t.Run("form", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
				t.Fatalf("Content-Type = %q, want form", ct)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if r.Form.Get("name") != "sdkit" {
				t.Fatalf("form name = %q, want sdkit", r.Form.Get("name"))
			}
		}))
		defer server.Close()

		_, err := request.Post(context.Background(), server.URL, request.WithFormMap(map[string]string{"name": "sdkit"}))
		if err != nil {
			t.Fatalf("Post() error = %v", err)
		}
	})

	t.Run("multipart", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reader, err := r.MultipartReader()
			if err != nil {
				t.Fatalf("MultipartReader() error = %v", err)
			}
			fields := readMultipart(t, reader)
			if fields["name"] != "sdkit" || fields["file"] != "content" {
				t.Fatalf("fields = %+v, want name and file content", fields)
			}
		}))
		defer server.Close()

		_, err := request.Post(
			context.Background(),
			server.URL,
			request.WithMultipart(
				request.FieldPart("name", "sdkit"),
				request.FilePart("file", "example.txt", "text/plain", strings.NewReader("content")),
			),
		)
		if err != nil {
			t.Fatalf("Post() error = %v", err)
		}
	})
}

func TestStreamLeavesBodyOpenForCaller(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flush")
		}
		_, _ = w.Write([]byte("chunk"))
		flusher.Flush()
	}))
	defer server.Close()

	stream, err := request.DefaultClient().Stream(context.Background(), http.MethodGet, server.URL)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()
	body, err := io.ReadAll(stream.Response.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(body) != "chunk" {
		t.Fatalf("stream body = %q, want chunk", body)
	}
}

func TestMaxBodyBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef"))
	}))
	defer server.Close()

	client, err := request.NewClient(request.WithMaxBodyBytes(3))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	resp, err := client.Get(context.Background(), server.URL)
	if !errors.Is(err, request.ErrBodyTooLarge) {
		t.Fatalf("Get() error = %v, want ErrBodyTooLarge", err)
	}
	if resp == nil || resp.String() != "abc" {
		t.Fatalf("body = %q, want truncated abc", resp.String())
	}
}

func TestContextDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := request.Get(ctx, server.URL)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Get() error = %v, want deadline exceeded", err)
	}
}

func TestHooks(t *testing.T) {
	var before int32
	var after int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Hook") != "before" {
			t.Fatalf("X-Hook = %q, want before", r.Header.Get("X-Hook"))
		}
	}))
	defer server.Close()

	client, err := request.NewClient(
		request.WithBeforeHook(func(ctx context.Context, req *http.Request) error {
			atomic.AddInt32(&before, 1)
			req.Header.Set("X-Hook", "before")
			return nil
		}),
		request.WithAfterHook(func(ctx context.Context, resp *request.Response, err error) error {
			atomic.AddInt32(&after, 1)
			if err != nil {
				t.Fatalf("after hook err = %v", err)
			}
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if _, err := client.Get(context.Background(), server.URL); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if before != 1 || after != 1 {
		t.Fatalf("before=%d after=%d, want 1/1", before, after)
	}
}

func TestRequestOptions(t *testing.T) {
	form := url.Values{}
	form.Set("name", "sdkit")
	if _, err := request.Post(context.Background(), "://bad-url", request.WithForm(form)); err == nil {
		t.Fatal("Post() error = nil, want bad url error")
	}
	if _, err := request.NewClient(request.WithBaseURL("/relative")); err == nil {
		t.Fatal("NewClient() error = nil, want base url validation error")
	}
}

func readMultipart(t *testing.T, reader *multipart.Reader) map[string]string {
	t.Helper()
	fields := make(map[string]string)
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return fields
		}
		if err != nil {
			t.Fatalf("NextPart() error = %v", err)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		fields[part.FormName()] = string(data)
	}
}
