package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/huwenlong92/sdkit/core/ratelimit/keyer"
	"github.com/huwenlong92/sdkit/core/ratelimit/middleware"
	"github.com/huwenlong92/sdkit/pkg/ratelimit/strategy"

	"github.com/gin-gonic/gin"
)

func TestLimiterPerUser_Int64UserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		keyer.SetSubject(c, "user", int64(1001))
		c.Next()
	})
	r.Use(middleware.LimiterPerUser(0.000001, 3))
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
	r.Use(middleware.LimiterPerUser(0.000001, 3))
	r.GET("/", func(c *gin.Context) { c.JSON(200, gin.H{}) })

	makeReq := func(uid string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		req.Header.Set("X-User-Id", uid)
		r.ServeHTTP(w, req)
		return w
	}

	for i := 0; i < 3; i++ {
		if makeReq("1001").Code != 200 {
			t.Fatalf("用户 1001 第%d次应该通过", i+1)
		}
	}
	if makeReq("1001").Code != 429 {
		t.Fatal("用户 1001 第4次应该 429")
	}

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
	r.Use(middleware.LimiterPerUser(0.000001, 3))
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

func TestLimiterPerUserRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		keyer.SetSubject(c, "user", int64(1001))
		c.Next()
	})
	r.Use(middleware.LimiterPerUserRoute(0.000001, 2))
	r.GET("/api/a", func(c *gin.Context) { c.JSON(200, gin.H{}) })
	r.GET("/api/b", func(c *gin.Context) { c.JSON(200, gin.H{}) })

	makeReq := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", path, nil)
		r.ServeHTTP(w, req)
		return w
	}

	if makeReq("/api/a").Code != 200 {
		t.Fatal("路由 A 第1次应该通过")
	}
	if makeReq("/api/a").Code != 200 {
		t.Fatal("路由 A 第2次应该通过")
	}
	if makeReq("/api/a").Code != 429 {
		t.Fatal("路由 A 第3次应该 429")
	}

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

// PerKey 测试
func TestPerKey_Isolation(t *testing.T) {
	tb1 := strategy.NewTokenBucket(100, 1)
	tb2 := strategy.NewTokenBucket(100, 2)

	if !tb1.Allow("client-a") {
		t.Fatal("client-a 第1次应该通过（tb1, burst=1）")
	}
	if tb1.Allow("client-a") {
		t.Fatal("client-a 第2次应该被限（tb1, burst=1）")
	}

	if !tb2.Allow("client-b") {
		t.Fatal("client-b 第1次应该通过（tb2, burst=2）")
	}
	if !tb2.Allow("client-b") {
		t.Fatal("client-b 第2次应该通过（tb2, burst=2）")
	}
}
