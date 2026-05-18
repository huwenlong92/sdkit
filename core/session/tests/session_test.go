package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/huwenlong92/sdkit/core/session"

	"github.com/gin-gonic/gin"
)

type testUser struct {
	ID   int64
	Name string
}

func TestMiddlewareSetGetAndHook(t *testing.T) {
	gin.SetMode(gin.TestMode)
	session.Register(testUser{})

	middleware, err := session.Middleware(session.Config{
		Type:   session.TypeCookie,
		Key:    "test_sid",
		Secret: "test-secret",
	})
	if err != nil {
		t.Fatalf("Middleware() error = %v", err)
	}

	called := false
	router := gin.New()
	router.Use(middleware)
	router.GET("/login", func(c *gin.Context) {
		err := session.Set(c, "user", testUser{ID: 1001, Name: "alice"}, session.Options{}, func(c *gin.Context, s session.Session) error {
			called = true
			return nil
		})
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		c.Status(http.StatusNoContent)
	})
	router.GET("/me", func(c *gin.Context) {
		value := session.Get(c, "user")
		user, ok := value.(testUser)
		if !ok || user.ID != 1001 {
			t.Fatalf("Get() = %#v, want testUser", value)
		}
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/login", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("login status = %d", w.Code)
	}
	if !called {
		t.Fatal("hook was not called")
	}

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	for _, cookie := range w.Result().Cookies() {
		req.AddCookie(cookie)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("me status = %d", w.Code)
	}
}

func TestDeleteAndClear(t *testing.T) {
	gin.SetMode(gin.TestMode)
	middleware, err := session.Middleware(session.Config{Key: "test_sid", Secret: "test-secret"})
	if err != nil {
		t.Fatalf("Middleware() error = %v", err)
	}

	router := gin.New()
	router.Use(middleware)
	router.GET("/set", func(c *gin.Context) {
		if err := session.Set(c, "a", "1", session.Options{}); err != nil {
			t.Fatalf("Set(a) error = %v", err)
		}
		if err := session.Set(c, "b", "2", session.Options{}); err != nil {
			t.Fatalf("Set(b) error = %v", err)
		}
		c.Status(http.StatusNoContent)
	})
	router.GET("/delete", func(c *gin.Context) {
		if err := session.Delete(c, "a", session.Options{}); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		if got := session.Get(c, "a"); got != nil {
			t.Fatalf("deleted value = %#v, want nil", got)
		}
		c.Status(http.StatusNoContent)
	})
	router.GET("/clear", func(c *gin.Context) {
		if err := session.Clear(c, session.Options{}); err != nil {
			t.Fatalf("Clear() error = %v", err)
		}
		if got := session.Get(c, "b"); got != nil {
			t.Fatalf("cleared value = %#v, want nil", got)
		}
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/set", nil))
	cookies := w.Result().Cookies()

	req := httptest.NewRequest(http.MethodGet, "/delete", nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/clear", nil)
	for _, cookie := range w.Result().Cookies() {
		req.AddCookie(cookie)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("clear status = %d", w.Code)
	}
}

func TestMiddlewareRequiresSecret(t *testing.T) {
	if _, err := session.Middleware(session.Config{}); err == nil {
		t.Fatal("Middleware() error = nil, want error")
	}
}
