package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/huwenlong92/sdkit/core/session"

	"github.com/gin-gonic/gin"
)

func TestStoreFromContext(t *testing.T) {
	store := session.NewMemoryStore()
	ctx := session.ContextWithStore(context.Background(), store)
	got, ok := session.FromContext(ctx)
	if !ok {
		t.Fatal("FromContext ok = false, want true")
	}
	if got != store {
		t.Fatalf("FromContext store = %v, want %v", got, store)
	}
}

func TestWithStoreMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := session.NewMemoryStore()
	router := gin.New()
	router.Use(session.WithStore(store))
	router.GET("/session", func(c *gin.Context) {
		got, ok := session.StoreFromContext(c.Request.Context())
		if !ok || got != store {
			t.Fatalf("StoreFromContext() = %v, %v; want store, true", got, ok)
		}
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/session", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}
