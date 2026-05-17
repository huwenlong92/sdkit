package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/session"

	"github.com/gin-gonic/gin"
)

func testStore(t *testing.T, store session.Store) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(session.Require(store))
	r.GET("/me", func(c *gin.Context) {
		sess := session.GetSession(c)
		c.JSON(200, gin.H{
			"subject_id":   sess.SubjectID,
			"subject_type": sess.SubjectType,
			"username":     sess.Username,
		})
	})

	// 1. 无 cookie → 401
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/me", nil)
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("无cookie: want 401, got %d", w.Code)
	}

	// 2. 创建 session
	sess := &session.Session{
		ID:          "test-sid-001",
		SubjectID:   1001,
		SubjectType: "admin",
		Username:    "admin",
		Extra: map[string]any{
			"role_id": int64(1),
		},
	}
	store.Set(req.Context(), sess, time.Minute)

	// 3. 带 cookie → 200
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/me", nil)
	req.AddCookie(&http.Cookie{Name: session.CookieName, Value: "test-sid-001"})
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("带cookie: want 200, got %d: %s", w.Code, w.Body.String())
	}

	// 4. session 过期
	store.Delete(req.Context(), "test-sid-001")
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/me", nil)
	req.AddCookie(&http.Cookie{Name: session.CookieName, Value: "test-sid-001"})
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("过期cookie: want 401, got %d", w.Code)
	}
}

func TestMemoryStore(t *testing.T) {
	store := session.NewMemoryStore()
	testStore(t, store)
}

func TestSessionCookie(t *testing.T) {
	store := session.NewMemoryStore()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/login", func(c *gin.Context) {
		sess := &session.Session{ID: "cookie-test", SubjectID: 1, SubjectType: "admin", Username: "test"}
		store.Set(c, sess, session.SessionTTL)
		session.SetCookie(c, sess.ID, session.SessionTTL)
		c.JSON(200, gin.H{"ok": true})
	})
	r.GET("/logout", func(c *gin.Context) {
		session.ClearCookie(c)
		c.JSON(200, gin.H{"ok": true})
	})

	// Login → 检查 Set-Cookie
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/login", nil)
	r.ServeHTTP(w, req)
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == session.CookieName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("登录响应应包含 sid cookie")
	}

	// Logout → cookie 被清除
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/logout", nil)
	r.ServeHTTP(w, req)
	for _, c := range w.Result().Cookies() {
		if c.Name == session.CookieName && c.MaxAge < 0 {
			return // pass
		}
	}
	t.Fatal("登出应清除 sid cookie (MaxAge < 0)")
}

func TestExtraField(t *testing.T) {
	store := session.NewMemoryStore()

	sess := &session.Session{
		ID:          "extra-test",
		SubjectID:   1,
		SubjectType: "admin",
		Username:    "test",
		Extra: map[string]any{
			"custom": "value",
			"count":  float64(42),
		},
	}
	ctx := context.Background()
	store.Set(ctx, sess, time.Minute)

	got, ok := store.Get(nil, "extra-test")
	if !ok {
		t.Fatal("应该能获取 session")
	}
	if got.Extra["custom"] != "value" {
		t.Fatalf("Extra.custom: want value, got %v", got.Extra["custom"])
	}
	if got.Extra["count"] != float64(42) {
		t.Fatalf("Extra.count: want 42, got %v", got.Extra["count"])
	}

	// 测试 GetExtra 辅助方法
	if v := got.GetExtraString("custom"); v != "value" {
		t.Fatalf("GetExtraString: want value, got %s", v)
	}
	if v, ok := got.GetExtraInt64("count"); !ok || v != 42 {
		t.Fatalf("GetExtraInt64: want 42, got %d", v)
	}
	if _, ok := got.GetExtra("nonexist"); ok {
		t.Fatal("GetExtra nonexist: should be false")
	}
}

// TestFullFlow 模拟 admin 登录 → 保护路由 → 登出
func TestFullFlow(t *testing.T) {
	store := session.NewMemoryStore()
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// 登录：创建 session + set cookie
	r.POST("/login", func(c *gin.Context) {
		sess := &session.Session{
			ID:          "sid-fullflow",
			SubjectID:   1001,
			SubjectType: "admin",
			Username:    "admin",
			Extra:       map[string]any{"role_id": int64(1)},
		}
		store.Set(c, sess, session.SessionTTL)
		session.SetCookie(c, sess.ID, session.SessionTTL)
		c.JSON(200, gin.H{"ok": true})
	})

	// 需要 session 保护
	r.GET("/me", session.Require(store), func(c *gin.Context) {
		sess := session.GetSession(c)
		c.JSON(200, gin.H{
			"subject_id":   sess.SubjectID,
			"subject_type": sess.SubjectType,
			"username":     sess.Username,
		})
	})

	// 登出
	r.POST("/logout", func(c *gin.Context) {
		sid, _ := c.Cookie(session.CookieName)
		store.Delete(c, sid)
		session.ClearCookie(c)
		c.JSON(200, gin.H{"ok": true})
	})

	// === 登录 ===
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/login", nil)
	r.ServeHTTP(w, req)
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("登录后应返回 sid cookie")
	}

	// === 访问保护路由 ===
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/me", nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("/me: want 200, got %d", w.Code)
	}

	// === 登出 ===
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/logout", nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	r.ServeHTTP(w, req)

	// === 登出后再访问 → 401 ===
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/me", nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("登出后: want 401, got %d", w.Code)
	}
}
