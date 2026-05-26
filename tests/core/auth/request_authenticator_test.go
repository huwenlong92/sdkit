package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/huwenlong92/sdkit/core/auth"
	authgin "github.com/huwenlong92/sdkit/core/auth/adapter/gin"
	"github.com/huwenlong92/sdkit/core/session"

	"github.com/gin-gonic/gin"
)

func TestJWTAuthenticatorExtractsBearerAndQueryToken(t *testing.T) {
	cfg := auth.JWTConfig{Secret: "secret", Issuer: "test", Expire: 3600}
	authenticator := auth.NewJWTAuthenticator(&cfg,
		auth.WithJWTProvider("api_jwt"),
		auth.WithJWTExtractor(auth.FirstExtractor(
			auth.QueryTokenExtractor("token"),
			auth.BearerTokenExtractor(),
		)),
	)
	login, err := authenticator.Login(context.Background(), &auth.Identity{
		SubjectID:   1001,
		SubjectType: "user",
		Username:    "demo",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/profile?token="+login.Token, nil)
	identity, err := authenticator.AuthenticateRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("AuthenticateRequest query: %v", err)
	}
	if identity.SubjectID != 1001 || identity.Method != auth.MethodJWT || identity.Provider != "api_jwt" {
		t.Fatalf("unexpected identity: %+v", identity)
	}

	req = httptest.NewRequest(http.MethodGet, "/profile", nil)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	identity, err = authenticator.AuthenticateRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("AuthenticateRequest bearer: %v", err)
	}
	if identity.SubjectKey() != "1001" {
		t.Fatalf("SubjectKey = %q, want 1001", identity.SubjectKey())
	}
}

func TestJWTAuthenticatorPreservesStringSubject(t *testing.T) {
	cfg := auth.JWTConfig{Secret: "secret", Issuer: "test", Expire: 3600}
	authenticator := auth.NewJWTAuthenticator(&cfg,
		auth.WithJWTExtractor(auth.BearerTokenExtractor()),
	)
	login, err := authenticator.Login(context.Background(), &auth.Identity{
		Subject:     "550e8400-e29b-41d4-a716-446655440000",
		SubjectType: "user",
		Username:    "uuid-user",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	identity, err := authenticator.AuthenticateRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("AuthenticateRequest: %v", err)
	}
	if identity.SubjectKey() != "550e8400-e29b-41d4-a716-446655440000" || identity.SubjectID != 0 {
		t.Fatalf("identity: %+v", identity)
	}
}

func TestGinSessionAuthenticatorReadsSessionIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	session.Register(sessionUser{})

	router := gin.New()
	middleware, err := session.Middleware(session.Config{Key: "web_sid", Secret: "test-secret"})
	if err != nil {
		t.Fatalf("session middleware: %v", err)
	}
	router.Use(middleware)

	authenticator := auth.NewSessionAuthenticator(auth.SessionAuthenticator{
		Provider: "web_session",
		Key:      "web_login",
		Reader:   authgin.SessionReader{},
		Mapper: func(_ context.Context, raw any) (*auth.Identity, error) {
			user, ok := raw.(sessionUser)
			if !ok {
				return nil, auth.ErrUnauthorized
			}
			return &auth.Identity{SubjectID: user.ID, SubjectType: "user", Username: user.Username}, nil
		},
	})

	router.GET("/login", func(c *gin.Context) {
		if err := session.Set(c, "web_login", sessionUser{ID: 7, Username: "web"}); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	router.GET("/me", authgin.Required(authenticator), func(c *gin.Context) {
		identity := authgin.GetIdentity(c)
		if identity == nil {
			c.Status(http.StatusUnauthorized)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"id":       identity.SubjectID,
			"provider": identity.Provider,
			"method":   identity.Method,
		})
	})

	loginResp := httptest.NewRecorder()
	router.ServeHTTP(loginResp, httptest.NewRequest(http.MethodGet, "/login", nil))
	if loginResp.Code != http.StatusNoContent {
		t.Fatalf("GET /login status = %d", loginResp.Code)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/me", nil)
	for _, cookie := range loginResp.Result().Cookies() {
		meReq.AddCookie(cookie)
	}
	meResp := httptest.NewRecorder()
	router.ServeHTTP(meResp, meReq)
	if meResp.Code != http.StatusOK {
		t.Fatalf("GET /me status = %d body=%s", meResp.Code, meResp.Body.String())
	}
}

func TestGinSessionAuthenticatorRunsLifecycleHooks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	session.Register(sessionUser{})

	router := gin.New()
	middleware, err := session.Middleware(session.Config{Key: "web_sid", Secret: "test-secret"})
	if err != nil {
		t.Fatalf("session middleware: %v", err)
	}
	router.Use(middleware)

	var validated bool
	var refreshed bool
	var failed bool
	authenticator := auth.NewSessionAuthenticator(auth.SessionAuthenticator{
		Provider: "web_session",
		Key:      "web_login",
		Reader:   authgin.SessionReader{},
		Mapper: func(_ context.Context, raw any) (*auth.Identity, error) {
			user, ok := raw.(sessionUser)
			if !ok {
				return nil, auth.ErrUnauthorized
			}
			return &auth.Identity{SubjectID: user.ID, SubjectType: "user", Username: user.Username}, nil
		},
		Validators: []auth.IdentityHook{
			func(_ context.Context, identity *auth.Identity, _ any) error {
				validated = identity.SubjectID == 7
				return nil
			},
		},
		Refreshers: []auth.IdentityHook{
			func(ctx context.Context, _ *auth.Identity, raw any) error {
				c, ok := authgin.ContextFrom(ctx)
				if !ok {
					return auth.ErrUnauthorized
				}
				user := raw.(sessionUser)
				user.Username = "refreshed"
				refreshed = true
				return session.Set(c, "web_login", user)
			},
		},
		Failures: []auth.IdentityFailureHook{
			func(context.Context, *http.Request, error) error {
				failed = true
				return nil
			},
		},
	})

	router.GET("/login", func(c *gin.Context) {
		if err := session.Set(c, "web_login", sessionUser{ID: 7, Username: "web"}); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	router.GET("/me", authgin.Required(authenticator), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET("/name", func(c *gin.Context) {
		user, _ := session.Get(c, "web_login").(sessionUser)
		c.String(http.StatusOK, user.Username)
	})

	loginResp := httptest.NewRecorder()
	router.ServeHTTP(loginResp, httptest.NewRequest(http.MethodGet, "/login", nil))

	meReq := httptest.NewRequest(http.MethodGet, "/me", nil)
	for _, cookie := range loginResp.Result().Cookies() {
		meReq.AddCookie(cookie)
	}
	meResp := httptest.NewRecorder()
	router.ServeHTTP(meResp, meReq)
	if meResp.Code != http.StatusNoContent {
		t.Fatalf("GET /me status = %d body=%s", meResp.Code, meResp.Body.String())
	}
	if !validated || !refreshed || failed {
		t.Fatalf("hooks validated=%v refreshed=%v failed=%v", validated, refreshed, failed)
	}

	nameReq := httptest.NewRequest(http.MethodGet, "/name", nil)
	for _, cookie := range meResp.Result().Cookies() {
		nameReq.AddCookie(cookie)
	}
	nameResp := httptest.NewRecorder()
	router.ServeHTTP(nameResp, nameReq)
	if nameResp.Body.String() != "refreshed" {
		t.Fatalf("refreshed session username = %q", nameResp.Body.String())
	}

	missingResp := httptest.NewRecorder()
	router.ServeHTTP(missingResp, httptest.NewRequest(http.MethodGet, "/me", nil))
	if missingResp.Code != http.StatusUnauthorized || !failed {
		t.Fatalf("missing session status=%d failed=%v", missingResp.Code, failed)
	}
}

type sessionUser struct {
	ID       int64
	Username string
}
