package tests

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/huwenlong92/sdkit/core/auth"
	authgin "github.com/huwenlong92/sdkit/core/auth/adapter/gin"

	"github.com/gin-gonic/gin"
)

type testUser struct {
	ID       int64
	Username string
	Password string
	RoleID   int64
}

func testHooks() auth.Hooks {
	user := &testUser{ID: 1001, Username: "admin", Password: "secret", RoleID: 1}
	return auth.HookFuncs{
		FindUserFunc: func(ctx context.Context, username string) (any, error) {
			if username != user.Username {
				return nil, auth.ErrInvalidCredentials
			}
			return user, nil
		},
		CheckPasswordFunc: func(ctx context.Context, raw any, password string) error {
			u, _ := raw.(*testUser)
			if u == nil || password != u.Password {
				return auth.ErrInvalidCredentials
			}
			return nil
		},
		BuildIdentityFunc: func(ctx context.Context, raw any) (*auth.Identity, error) {
			u, _ := raw.(*testUser)
			if u == nil {
				return nil, auth.ErrInvalidCredentials
			}
			return &auth.Identity{
				SubjectID:   u.ID,
				SubjectType: "admin",
				Username:    u.Username,
				RoleID:      u.RoleID,
				Permissions: []string{"system:read"},
				Extra:       map[string]any{"role_id": u.RoleID},
			}, nil
		},
	}
}

func setup(a *auth.Auth) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	r.POST("/login", func(c *gin.Context) {
		result, err := authgin.LoginByCredentials(c, a, auth.Credentials{
			Username: c.Query("username"),
			Password: c.Query("password"),
		})
		if err != nil {
			c.JSON(401, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"token": result.Token, "mode": result.Mode})
	})

	r.POST("/logout", func(c *gin.Context) {
		_ = authgin.Logout(c, a)
		c.JSON(200, gin.H{"ok": true})
	})

	group := r.Group("")
	group.Use(authgin.AuthMiddleware(a))
	group.GET("/me", func(c *gin.Context) {
		identity := authgin.GetIdentity(c)
		c.JSON(200, gin.H{
			"subject_id":   identity.SubjectID,
			"subject_type": identity.SubjectType,
			"username":     identity.Username,
			"role_id":      identity.RoleID,
		})
	})

	return r
}

func TestJWTGuardLoginAndMiddleware(t *testing.T) {
	a := auth.NewWithGuard(
		auth.NewJWTGuard(&auth.JWTConfig{Secret: "test-secret", Issuer: "test", Expire: 3600}),
		testHooks(),
	)
	r := setup(a)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/login?username=admin&password=secret", nil)
	r.ServeHTTP(w, req)
	token := getJSONField(t, w.Body.String(), "token")

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("JWT: want 200, got %d", w.Code)
	}
}

func TestLoginFailedHook(t *testing.T) {
	called := false
	hooks := testHooks().(auth.HookFuncs)
	hooks.AfterLoginFailedFunc = func(ctx context.Context, username string, err error) {
		called = true
		if username != "admin" || !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("unexpected failed hook args: username=%s err=%v", username, err)
		}
	}
	a := auth.NewWithGuard(
		auth.NewJWTGuard(&auth.JWTConfig{Secret: "test-secret", Issuer: "test", Expire: 3600}),
		hooks,
	)
	r := setup(a)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/login?username=admin&password=bad", nil)
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("login failed: want 401, got %d", w.Code)
	}
	if !called {
		t.Fatal("AfterLoginFailed should be called")
	}
}

func getJSONField(t *testing.T, body, key string) string {
	t.Helper()
	body = body[1 : len(body)-1]
	for _, pair := range strings.Split(body, ",") {
		kv := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.Trim(kv[0], `"`)
		v := strings.Trim(kv[1], `"`)
		if k == key {
			return v
		}
	}
	t.Fatalf("key %q not found in %s", key, body)
	return ""
}
