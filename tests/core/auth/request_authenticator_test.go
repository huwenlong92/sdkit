package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	coreauth "github.com/huwenlong92/sdkit/core/auth"
	authgin "github.com/huwenlong92/sdkit/core/auth/adapter/gin"
	"github.com/huwenlong92/sdkit/core/session"

	"github.com/gin-gonic/gin"
)

func TestJWTAuthenticatorExtractsBearerAndQueryToken(t *testing.T) {
	cfg := coreauth.JWTConfig{Secret: "secret", Issuer: "test", Expire: 3600}
	authenticator := coreauth.NewJWTAuthenticator(&cfg,
		coreauth.WithJWTProvider("api_jwt"),
		coreauth.WithJWTExtractor(coreauth.FirstExtractor(
			coreauth.QueryTokenExtractor("token"),
			coreauth.BearerTokenExtractor(),
		)),
	)
	login, err := authenticator.Login(context.Background(), &coreauth.Identity{
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
	if identity.SubjectID != 1001 || identity.Method != coreauth.MethodJWT || identity.Provider != "api_jwt" {
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

func TestGinSessionAuthenticatorReadsSessionIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	session.Register(sessionUser{})

	router := gin.New()
	middleware, err := session.Middleware(session.Config{Key: "web_sid", Secret: "test-secret"})
	if err != nil {
		t.Fatalf("session middleware: %v", err)
	}
	router.Use(middleware)

	authenticator := coreauth.NewSessionAuthenticator(coreauth.SessionAuthenticator{
		Provider: "web_session",
		Key:      "web_login",
		Reader:   authgin.SessionReader{},
		Mapper: func(_ context.Context, raw any) (*coreauth.Identity, error) {
			user, ok := raw.(sessionUser)
			if !ok {
				return nil, coreauth.ErrUnauthorized
			}
			return &coreauth.Identity{SubjectID: user.ID, SubjectType: "user", Username: user.Username}, nil
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

type sessionUser struct {
	ID       int64
	Username string
}
