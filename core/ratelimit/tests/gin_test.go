package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	rlMiddleware "github.com/huwenlong92/sdkit/core/ratelimit/middleware"
	"github.com/huwenlong92/sdkit/pkg/ratelimit/strategy"

	"github.com/gin-gonic/gin"
)

func TestLimiterPerIP_Presets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		fn   func() gin.HandlerFunc
	}{
		{"Loose", rlMiddleware.LimiterLoose},
		{"Normal", rlMiddleware.LimiterNormal},
		{"Strict", rlMiddleware.LimiterStrict},
		{"Upload", rlMiddleware.LimiterUpload},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(tt.fn())
			r.GET("/", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/", nil)
			r.ServeHTTP(w, req)
			if w.Code != 200 {
				t.Fatalf("%s: want 200, got %d", tt.name, w.Code)
			}
		})
	}
}

func TestLimiterPerIP_429(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(rlMiddleware.Limiter(0.000001, 1))
	r.GET("/", func(c *gin.Context) { c.JSON(200, gin.H{}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("第1次: want 200, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatalf("第2次: want 429, got %d", w.Code)
	}
}

func TestLimiterLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(rlMiddleware.LimiterLogin())
	r.GET("/login", func(c *gin.Context) { c.JSON(200, gin.H{}) })

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/login", nil)
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("第%d次: want 200, got %d", i+1, w.Code)
		}
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/login", nil)
	r.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatalf("第6次: want 429, got %d", w.Code)
	}
}

func TestMiddleware_LimiterInterface(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tb := strategy.NewTokenBucket(100, 1)
	r := gin.New()
	r.Use(rlMiddleware.Middleware(tb))
	r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("第1次请求应该 200, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatalf("第2次请求应该 429, got %d", w.Code)
	}
}

func TestMiddlewareWithKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tb := strategy.NewTokenBucket(100, 1)
	r := gin.New()
	r.Use(rlMiddleware.MiddlewareWithKey(tb, func(c *gin.Context) string {
		return c.GetHeader("X-User-Id")
	}))
	r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	// User A 第一次
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-User-Id", "user-a")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("user-a 第1次应该 200, got %d", w.Code)
	}

	// User A 第二次被拒
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-User-Id", "user-a")
	r.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatalf("user-a 第2次应该 429, got %d", w.Code)
	}

	// User B 不受影响
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-User-Id", "user-b")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("user-b 第1次应该 200, got %d", w.Code)
	}
}

func TestBBRMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(rlMiddleware.BBRNormal())
	r.GET("/test", func(c *gin.Context) {
		time.Sleep(5 * time.Millisecond)
		c.JSON(200, gin.H{"ok": true})
	})

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("BBR normal request %d: want 200, got %d", i+1, w.Code)
		}
	}
}
