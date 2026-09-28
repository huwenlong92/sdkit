package jwtx_test

import (
	"testing"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/huwenlong92/sdkit/pkg/jwtx"
)

func TestSubjectClaimsRoundTripExtendedFieldsAndAudience(t *testing.T) {
	token, err := jwtx.SignSubject("test-secret", "test-issuer", 60, jwtx.Subject{
		ID:          42,
		Subject:     "app_demo",
		Type:        "application",
		TenantID:    7,
		Roles:       []string{"client"},
		Permissions: []string{"job:read"},
		Extra: map[string]any{
			"credential_version": "3",
		},
		Audience: []string{"open-api"},
	})
	if err != nil {
		t.Fatalf("SignSubject: %v", err)
	}

	claims, err := jwtx.ParseSubjectWithOptions(token, "test-secret",
		jwt.WithAudience("open-api"),
		jwt.WithIssuer("test-issuer"),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		t.Fatalf("ParseSubjectWithOptions: %v", err)
	}
	if claims.SubjectID != 42 || claims.Subject != "app_demo" || claims.TenantID != 7 {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "client" || claims.Extra["credential_version"] != "3" {
		t.Fatalf("extended claims not preserved: %+v", claims)
	}

	if _, err := jwtx.ParseSubjectWithOptions(token, "test-secret", jwt.WithAudience("another-service")); err == nil {
		t.Fatal("wrong audience must fail")
	}
}

func TestParseSubjectRemainsCompatibleWithoutValidationOptions(t *testing.T) {
	token, err := jwtx.SignSubject("test-secret", "test-issuer", 60, jwtx.Subject{ID: 1, Type: "user"})
	if err != nil {
		t.Fatalf("SignSubject: %v", err)
	}
	if _, err := jwtx.ParseSubject(token, "test-secret"); err != nil {
		t.Fatalf("ParseSubject: %v", err)
	}
}
