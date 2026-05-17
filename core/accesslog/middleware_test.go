package accesslog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestSummarizeJSONRedactsSensitiveFields(t *testing.T) {
	body := []byte(`{
		"username":"admin",
		"password":"secret",
		"profile":{"access_token":"token-value"},
		"sessions":[{"client_secret":"client-secret"}]
	}`)

	got := string(summarizeJSON(body))
	if !strings.Contains(got, `"password":"(redacted)"`) {
		t.Fatalf("password should be redacted: %s", got)
	}
	if !strings.Contains(got, `"access_token":"(redacted)"`) {
		t.Fatalf("nested token should be redacted: %s", got)
	}
	if !strings.Contains(got, `"client_secret":"(redacted)"`) {
		t.Fatalf("nested secret should be redacted: %s", got)
	}
	if strings.Contains(got, "token-value") || strings.Contains(got, "client-secret") {
		t.Fatalf("sensitive values should not remain: %s", got)
	}
}

func TestMiddlewareSeparatesTrackIDAndTraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	oldProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(oldProvider)
	defer provider.Shutdown(context.Background())

	writer := &captureWriter{ch: make(chan *Entry, 1)}
	logCtx, stop := context.WithCancel(context.Background())
	accessLogger := NewLogger(writer, Config{BatchSize: 1, FlushInterval: time.Hour})
	accessLogger.Start(logCtx)
	defer stop()

	r := gin.New()
	r.Use(tracking.Middleware())
	r.Use(tracing.Middleware("accesslog-test"))
	r.Use(requestid.Middleware())
	r.Use(Middleware("test", WithLogger(accessLogger)))
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(tracking.Header, "track-accesslog")
	req.Header.Set(requestid.Header, "request-accesslog")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	entry := waitEntry(t, writer.ch)
	if entry.TrackID != "track-accesslog" {
		t.Fatalf("track_id = %q, want track-accesslog", entry.TrackID)
	}
	if entry.RequestID != "request-accesslog" {
		t.Fatalf("request_id = %q, want request-accesslog", entry.RequestID)
	}
	if entry.TraceID == "" {
		t.Fatal("trace_id should not be empty when tracing middleware is registered")
	}
	if entry.TraceID == entry.TrackID {
		t.Fatalf("trace_id should not reuse track_id: trace_id=%q track_id=%q", entry.TraceID, entry.TrackID)
	}
}

type captureWriter struct {
	ch chan *Entry
}

func (w *captureWriter) WriteBatch(ctx context.Context, entries []*Entry) error {
	for _, entry := range entries {
		w.ch <- entry
	}
	return nil
}

func waitEntry(t *testing.T, ch <-chan *Entry) *Entry {
	t.Helper()
	select {
	case entry := <-ch:
		return entry
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for accesslog entry")
		return nil
	}
}
