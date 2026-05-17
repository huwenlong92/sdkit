package tests

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestMiddlewareCreatesHTTPRootSpanAndKeepsTrackID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	oldProvider := otel.GetTracerProvider()
	oldPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(tracing.NewPropagator())
	defer otel.SetTracerProvider(oldProvider)
	defer otel.SetTextMapPropagator(oldPropagator)
	defer provider.Shutdown(context.Background())

	r := gin.New()
	r.Use(tracking.Middleware())
	r.Use(tracing.Middleware("test-api"))
	r.Use(requestid.Middleware())
	r.GET("/ping", func(c *gin.Context) {
		if tracing.TraceID(c.Request.Context()) == "" {
			t.Fatal("trace_id should not be empty")
		}
		if tracing.SpanID(c.Request.Context()) == "" {
			t.Fatal("span_id should not be empty")
		}
		if tracking.TrackID(c.Request.Context()) != "track-http" {
			t.Fatalf("track_id: want track-http, got %s", tracking.TrackID(c.Request.Context()))
		}
		if requestID := requestid.FromContext(c.Request.Context()); requestID != "request-http" {
			t.Fatalf("request_id: want request-http, got %s", requestID)
		}
		c.Status(204)
	})

	req := httptest.NewRequest("GET", "/ping", nil)
	req.Header.Set(tracking.Header, "track-http")
	req.Header.Set(requestid.Header, "request-http")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get(tracking.Header); got != "track-http" {
		t.Fatalf("track response header: want track-http, got %s", got)
	}
	if got := w.Header().Get(requestid.Header); got != "request-http" {
		t.Fatalf("request response header: want request-http, got %s", got)
	}
	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans: want 1, got %d", len(ended))
	}
	if ended[0].Name() != "GET /ping" {
		t.Fatalf("span name: want GET /ping, got %s", ended[0].Name())
	}
	assertHTTPSpanAttr(t, ended[0], "trace_id", ended[0].SpanContext().TraceID().String())
	assertHTTPSpanAttr(t, ended[0], "span_id", ended[0].SpanContext().SpanID().String())
	assertHTTPSpanAttr(t, ended[0], "track_id", "track-http")
	assertHTTPSpanAttr(t, ended[0], "request_id", "request-http")
	assertHTTPSpanAttr(t, ended[0], "sd.track_id", "track-http")
	assertHTTPSpanAttr(t, ended[0], "sd.request_id", "request-http")
	assertHTTPSpanAttrNotEmpty(t, ended[0], "traceparent")
}

func TestMiddlewareRecordsServerErrorStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	oldProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(oldProvider)
	defer provider.Shutdown(context.Background())

	r := gin.New()
	r.Use(tracing.Middleware("test-api"))
	r.GET("/fail", func(c *gin.Context) {
		c.Status(500)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/fail", nil))

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans: want 1, got %d", len(ended))
	}
	if ended[0].Status().Code.String() != "Error" {
		t.Fatalf("span status: want Error, got %s", ended[0].Status().Code.String())
	}
}

func assertHTTPSpanAttr(t *testing.T, span sdktrace.ReadOnlySpan, key string, want string) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key && attr.Value.AsString() == want {
			return
		}
	}
	t.Fatalf("missing span attr %s=%s: %+v", key, want, span.Attributes())
}

func assertHTTPSpanAttrNotEmpty(t *testing.T, span sdktrace.ReadOnlySpan, key string) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key && attr.Value.AsString() != "" {
			return
		}
	}
	t.Fatalf("missing non-empty span attr %s: %+v", key, span.Attributes())
}
