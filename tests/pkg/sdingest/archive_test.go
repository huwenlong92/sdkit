package sdingest_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/pkg/sdingest"
)

func TestArchiveClientCoversRESTContract(t *testing.T) {
	seen := make([]string, 0, 8)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/auth/token" {
			writeEnvelope(t, writer, http.StatusOK, http.StatusOK, "", map[string]any{"access_token": "token", "expires_in": 3600})
			return
		}
		seen = append(seen, request.Method+" "+request.URL.Path)
		switch request.URL.Path {
		case "/v1/archive/job/create":
			if request.Header.Get("Idempotency-Key") != "archive:create:42" {
				t.Errorf("create idempotency key = %q", request.Header.Get("Idempotency-Key"))
			}
			var input sdingest.CreateArchiveJobInput
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatalf("decode create: %v", err)
			}
			if input.CallbackURL != "http://callback.example.test/archive" || input.OutputTargetID != "target-out" || len(input.Entries) != 2 || input.Entries[1].TargetID != "target-b" {
				t.Errorf("create input = %+v", input)
			}
			writeArchiveJob(t, writer, "arc-42", "pending")
		case "/v1/archive/job/list":
			if request.URL.Query().Get("status") != "running" || request.URL.Query().Get("search") != "delivery" || request.URL.Query().Get("page") != "2" {
				t.Errorf("list query = %s", request.URL.RawQuery)
			}
			writeEnvelope(t, writer, http.StatusOK, http.StatusOK, "", map[string]any{"list": []any{}, "total": 0})
		case "/v1/archive/job/detail":
			writeArchiveJob(t, writer, request.URL.Query().Get("job_id"), "running")
		case "/v1/archive/job/manifest":
			writeEnvelope(t, writer, http.StatusOK, http.StatusOK, "", map[string]any{
				"job":     archiveJobData("arc-42", "running"),
				"entries": []map[string]any{{"entry_id": "entry-1", "sequence": 1, "target_id": "target-a", "source_path": "source/a.mp4", "archive_path": "video/a.mp4", "status": "succeeded", "size": 128, "processed_bytes": 128, "progress_percent": 100}},
				"total":   1,
			})
		case "/v1/archive/job/callback-logs":
			writeEnvelope(t, writer, http.StatusOK, http.StatusOK, "", map[string]any{"list": []map[string]any{{"event_id": "evt-1", "event": "archive.succeeded", "callback_url": "http://callback.example.test/archive", "attempt_no": 1, "status": "succeeded", "latency_ms": 8}}, "total": 1})
		case "/v1/archive/job/callback-log-detail":
			if request.URL.Query().Get("event_id") != "evt-1" || request.URL.Query().Get("attempt_no") != "1" {
				t.Errorf("callback detail query = %s", request.URL.RawQuery)
			}
			writeEnvelope(t, writer, http.StatusOK, http.StatusOK, "", map[string]any{"event_id": "evt-1", "event": "archive.succeeded", "callback_url": "http://callback.example.test/archive", "job_id": "arc-42", "attempt_no": 1, "status": "succeeded", "payload": map[string]any{"event_id": "evt-1"}})
		case "/v1/archive/job/access":
			if request.URL.Query().Get("ttl_seconds") != "900" {
				t.Errorf("access query = %s", request.URL.RawQuery)
			}
			writeEnvelope(t, writer, http.StatusOK, http.StatusOK, "", map[string]any{"job_id": "arc-42", "target_id": "target-out", "filename": "delivery.zip", "path": "archives/delivery.zip", "object_uri": "s3://bucket/archives/delivery.zip", "url": "https://storage.example.test/delivery.zip", "expires_at": time.Now().UTC()})
		case "/v1/archive/job/retry":
			if request.Header.Get("Idempotency-Key") != "archive:retry:42" {
				t.Errorf("retry idempotency key = %q", request.Header.Get("Idempotency-Key"))
			}
			writeArchiveJob(t, writer, "arc-42", "pending")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := newClient(t, server.URL)
	ctx := context.Background()
	created, err := client.CreateArchiveJob(ctx, sdingest.CreateArchiveJobInput{
		ExternalRef: "delivery-42", CallbackURL: "http://callback.example.test/archive", ArchiveName: "delivery.zip",
		RootDirectory: "交付", OutputTargetID: "target-out",
		Entries: []sdingest.ArchiveEntryInput{{TargetID: "target-a", SourcePath: "source/a.mp4", ArchivePath: "video/a.mp4", Size: 128}, {TargetID: "target-b", SourcePath: "source/b.srt", ArchivePath: "subtitle/b.srt", Size: 32}},
	}, "archive:create:42")
	if err != nil || created.JobID != "arc-42" || created.CallbackURL == "" {
		t.Fatalf("CreateArchiveJob() = %+v, %v", created, err)
	}
	if _, err := client.ListArchiveJobs(ctx, sdingest.ListArchiveJobsInput{Page: 2, Limit: 20, Status: "running", Search: "delivery"}); err != nil {
		t.Fatal(err)
	}
	detail, err := client.GetArchiveJob(ctx, "arc-42")
	if err != nil || detail.Status != sdingest.ArchiveJobStatusRunning || detail.Terminal() {
		t.Fatalf("GetArchiveJob() = %+v, %v", detail, err)
	}
	manifest, err := client.GetArchiveManifest(ctx, "arc-42", 1, 50)
	if err != nil || len(manifest.Entries) != 1 || manifest.Entries[0].ArchivePath != "video/a.mp4" {
		t.Fatalf("GetArchiveManifest() = %+v, %v", manifest, err)
	}
	logs, err := client.ListArchiveCallbackLogs(ctx, "arc-42", 1, 20)
	if err != nil || len(logs.List) != 1 || logs.List[0].Event != sdingest.ArchiveCallbackEventSucceeded {
		t.Fatalf("ListArchiveCallbackLogs() = %+v, %v", logs, err)
	}
	logDetail, err := client.GetArchiveCallbackLog(ctx, "arc-42", "evt-1", 1)
	if err != nil || string(logDetail.Payload) == "" || logDetail.JobID != "arc-42" {
		t.Fatalf("GetArchiveCallbackLog() = %+v, %v", logDetail, err)
	}
	access, err := client.GetArchiveAccess(ctx, "arc-42", 15*time.Minute)
	if err != nil || access.TargetID != "target-out" || access.Path != "archives/delivery.zip" {
		t.Fatalf("GetArchiveAccess() = %+v, %v", access, err)
	}
	retried, err := client.RetryArchiveJob(ctx, "arc-42", "archive:retry:42")
	if err != nil || retried.JobID != "arc-42" {
		t.Fatalf("RetryArchiveJob() = %+v, %v", retried, err)
	}
	want := []string{
		"POST /v1/archive/job/create", "GET /v1/archive/job/list", "GET /v1/archive/job/detail",
		"GET /v1/archive/job/manifest", "GET /v1/archive/job/callback-logs", "GET /v1/archive/job/callback-log-detail",
		"GET /v1/archive/job/access", "POST /v1/archive/job/retry",
	}
	if !slices.Equal(seen, want) {
		t.Fatalf("routes = %v, want %v", seen, want)
	}
}

func TestArchiveWritesRequireStableIdempotencyKeys(t *testing.T) {
	client := &sdingest.Client{}
	if _, err := client.CreateArchiveJob(context.Background(), sdingest.CreateArchiveJobInput{}, ""); !errors.Is(err, sdingest.ErrIdempotencyKeyRequired) {
		t.Fatalf("CreateArchiveJob error = %v", err)
	}
	if _, err := client.RetryArchiveJob(context.Background(), "arc-1", ""); !errors.Is(err, sdingest.ErrIdempotencyKeyRequired) {
		t.Fatalf("RetryArchiveJob error = %v", err)
	}
}

func TestStreamArchiveJobEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/auth/token":
			writeEnvelope(t, writer, http.StatusOK, http.StatusOK, "", map[string]any{"access_token": "stream-token", "expires_in": 3600})
		case "/v1/archive/job/events":
			if request.Header.Get("Authorization") != "Bearer stream-token" || request.Header.Get("Accept") != "text/event-stream" || request.URL.Query().Get("job_id") != "arc-stream" {
				t.Errorf("unexpected stream request: headers=%v query=%s", request.Header, request.URL.RawQuery)
			}
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(writer, "event: snapshot\ndata: {\"job_id\":\"arc-stream\",\"status\":\"running\",\"phase\":\"compressing\"}\n\n")
			_, _ = fmt.Fprint(writer, ": heartbeat\n\n")
			_, _ = fmt.Fprint(writer, "event: complete\ndata: {\"job_id\":\"arc-stream\",\"status\":\"succeeded\",\"phase\":\"completed\"}\n\n")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := newClient(t, server.URL)
	events := make([]sdingest.ArchiveJobEvent, 0, 2)
	err := client.StreamArchiveJobEvents(context.Background(), "arc-stream", func(event sdingest.ArchiveJobEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamArchiveJobEvents: %v", err)
	}
	if len(events) != 2 || events[0].Event != "snapshot" || events[1].Event != "complete" || !events[1].Job.Terminal() {
		t.Fatalf("events = %+v", events)
	}
}

func TestStreamArchiveJobEventsRefreshesUnauthorizedTokenOnce(t *testing.T) {
	var tokenRequests atomic.Int64
	var streamRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/auth/token":
			count := tokenRequests.Add(1)
			writeEnvelope(t, writer, http.StatusOK, http.StatusOK, "", map[string]any{"access_token": fmt.Sprintf("token-%d", count), "expires_in": 3600})
		case "/v1/archive/job/events":
			streamRequests.Add(1)
			if request.Header.Get("Authorization") == "Bearer token-1" {
				writeEnvelope(t, writer, http.StatusUnauthorized, http.StatusUnauthorized, "TOKEN_EXPIRED", nil)
				return
			}
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(writer, "event: complete\ndata: {\"job_id\":\"arc-stream\",\"status\":\"succeeded\",\"phase\":\"completed\"}\n\n")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := newClient(t, server.URL)
	err := client.StreamArchiveJobEvents(context.Background(), "arc-stream", func(sdingest.ArchiveJobEvent) error { return nil })
	if err != nil {
		t.Fatalf("StreamArchiveJobEvents: %v", err)
	}
	if tokenRequests.Load() != 2 || streamRequests.Load() != 2 {
		t.Fatalf("token requests = %d, stream requests = %d", tokenRequests.Load(), streamRequests.Load())
	}
}

func archiveJobData(jobID string, status string) map[string]any {
	return map[string]any{
		"job_id": jobID, "app_id": "app-id", "callback_url": "http://callback.example.test/archive",
		"archive_name": "delivery.zip", "output_target_id": "target-out", "manifest_hash": "hash",
		"status": status, "phase": "compressing", "revision": 1, "entry_total": 2,
		"app":           map[string]any{"app_id": "app-id", "name": "app", "status": "enabled"},
		"output_target": map[string]any{"target_id": "target-out", "name": "output", "driver": "s3", "bucket": "bucket"},
	}
}

func writeArchiveJob(t *testing.T, writer http.ResponseWriter, jobID string, status string) {
	t.Helper()
	writeEnvelope(t, writer, http.StatusOK, http.StatusOK, "", archiveJobData(jobID, status))
}
