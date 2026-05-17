package requestid

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMiddlewareWritesRequestIDToRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware())
	r.GET("/", func(c *gin.Context) {
		requestID := FromContext(c.Request.Context())
		if requestID != "request-1" {
			t.Fatalf("request_id in request context: want request-1, got %v", requestID)
		}
		c.Status(204)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(Header, "request-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Header().Get(Header) != "request-1" {
		t.Fatalf("response header %s: want request-1, got %s", Header, w.Header().Get(Header))
	}
}

func TestWithRequestIDAndFromContext(t *testing.T) {
	ctx := WithRequestID(context.Background(), "request-typed")
	if got := FromContext(ctx); got != "request-typed" {
		t.Fatalf("request_id: want request-typed, got %s", got)
	}
}
