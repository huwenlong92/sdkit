package tests

import (
	"net/http/httptest"
	"testing"

	gintracking "github.com/huwenlong92/sdkit/core/gin/tracking"
	"github.com/huwenlong92/sdkit/core/tracking"

	"github.com/gin-gonic/gin"
)

func TestMiddlewarePassesThroughTrackID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gintracking.Middleware())
	r.GET("/", func(c *gin.Context) {
		if got := tracking.TrackID(c.Request.Context()); got != "track-1" {
			t.Fatalf("context track_id: want track-1, got %s", got)
		}
		if got := gintracking.Get(c); got != "track-1" {
			t.Fatalf("gin track_id: want track-1, got %s", got)
		}
		c.Status(204)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(tracking.Header, "track-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get(tracking.Header); got != "track-1" {
		t.Fatalf("response header %s: want track-1, got %s", tracking.Header, got)
	}
}

func TestMiddlewareGeneratesTrackID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gintracking.Middleware())
	r.GET("/", func(c *gin.Context) {
		if got := tracking.TrackID(c.Request.Context()); got == "" {
			t.Fatal("context track_id should not be empty")
		}
		c.Status(204)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))

	if got := w.Header().Get(tracking.Header); got == "" {
		t.Fatalf("response header %s should not be empty", tracking.Header)
	}
}

func TestMiddlewareForceNewTrackID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gintracking.Middleware(tracking.Config{Enabled: true, ForceNew: true}))
	r.GET("/", func(c *gin.Context) {
		if got := tracking.TrackID(c.Request.Context()); got == "" || got == "track-old" {
			t.Fatalf("context track_id should be new, got %s", got)
		}
		c.Status(204)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(tracking.Header, "track-old")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get(tracking.Header); got == "" || got == "track-old" {
		t.Fatalf("response header should be new, got %s", got)
	}
}

func TestMiddlewareDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gintracking.Middleware(tracking.Config{Enabled: false}))
	r.GET("/", func(c *gin.Context) {
		if got := tracking.TrackID(c.Request.Context()); got != "" {
			t.Fatalf("context track_id should be empty, got %s", got)
		}
		c.Status(204)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))

	if got := w.Header().Get(tracking.Header); got != "" {
		t.Fatalf("response header should be empty, got %s", got)
	}
}
