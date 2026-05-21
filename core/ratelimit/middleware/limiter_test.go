package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/huwenlong92/sdkit/core/ratelimit/keyer"

	"github.com/gin-gonic/gin"
)

func TestLimiterPerIP_Presets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		fn   func() gin.HandlerFunc
	}{
		{"Loose", LimiterLoose},
		{"Normal", LimiterNormal},
		{"Strict", LimiterStrict},
		{"Upload", LimiterUpload},
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

	// 极小 rate 确保 burst 用完后必然被限
	r := gin.New()
	r.Use(Limiter(0.000001, 1))
	r.GET("/", func(c *gin.Context) { c.JSON(200, gin.H{}) })

	// 第1次通过
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("第1次: want 200, got %d", w.Code)
	}

	// 第2次 429
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatalf("第2次: want 429, got %d", w.Code)
	}
}

func TestLimiterPerUser_Int64UserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		keyer.SetSubject(c, "user", int64(1001))
		c.Next()
	})
	r.Use(LimiterPerUser(0.000001, 3))
	r.GET("/", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("用户 1001 第%d次: want 200, got %d", i+1, w.Code)
		}
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatalf("第4次: want 429, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Fatal("429 响应体不应为空")
	}
}

func TestLimiterPerUser_DifferentUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		uid := c.GetHeader("X-User-Id")
		if uid == "1001" {
			keyer.SetSubject(c, "user", int64(1001))
		} else if uid == "1002" {
			keyer.SetSubject(c, "user", int64(1002))
		}
		c.Next()
	})
	r.Use(LimiterPerUser(0.000001, 3))
	r.GET("/", func(c *gin.Context) { c.JSON(200, gin.H{}) })

	makeReq := func(uid string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		req.Header.Set("X-User-Id", uid)
		r.ServeHTTP(w, req)
		return w
	}

	// 用户 1001 前 3 次通过
	for i := 0; i < 3; i++ {
		if makeReq("1001").Code != 200 {
			t.Fatalf("用户 1001 第%d次应该通过", i+1)
		}
	}
	if makeReq("1001").Code != 429 {
		t.Fatal("用户 1001 第4次应该 429")
	}

	// 用户 1002 前 3 次通过（独立计数）
	for i := 0; i < 3; i++ {
		if makeReq("1002").Code != 200 {
			t.Fatalf("用户 1002 第%d次应该通过（独立于 1001）", i+1)
		}
	}
	if makeReq("1002").Code != 429 {
		t.Fatal("用户 1002 第4次应该 429")
	}
}

func TestLimiterPerUser_NoUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(LimiterPerUser(0.000001, 3))
	r.GET("/", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	for i := 0; i < 20; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("无用户时第%d次应该放行, got %d", i+1, w.Code)
		}
	}
}

func TestLimiterLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(LimiterLogin())
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

func TestLimiterPerUserRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		keyer.SetSubject(c, "user", int64(1001))
		c.Next()
	})
	r.Use(LimiterPerUserRoute(0.000001, 2))
	r.GET("/api/a", func(c *gin.Context) { c.JSON(200, gin.H{}) })
	r.GET("/api/b", func(c *gin.Context) { c.JSON(200, gin.H{}) })

	makeReq := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", path, nil)
		r.ServeHTTP(w, req)
		return w
	}

	// 路由 A：前 2 次通过
	if makeReq("/api/a").Code != 200 {
		t.Fatal("路由 A 第1次应该通过")
	}
	if makeReq("/api/a").Code != 200 {
		t.Fatal("路由 A 第2次应该通过")
	}
	// 路由 A 第 3 次被限
	if makeReq("/api/a").Code != 429 {
		t.Fatal("路由 A 第3次应该 429")
	}

	// 路由 B：不受 A 影响，独立计数
	if makeReq("/api/b").Code != 200 {
		t.Fatal("路由 B 第1次应该通过（独立于路由 A）")
	}
	if makeReq("/api/b").Code != 200 {
		t.Fatal("路由 B 第2次应该通过")
	}
	if makeReq("/api/b").Code != 429 {
		t.Fatal("路由 B 第3次应该 429")
	}
}
