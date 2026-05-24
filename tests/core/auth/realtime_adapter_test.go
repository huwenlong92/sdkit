package auth_test

import (
	"context"
	"net/http"
	"testing"

	coreauth "github.com/huwenlong92/sdkit/core/auth"
	authrealtime "github.com/huwenlong92/sdkit/core/auth/adapter/realtime"
)

func TestRealtimeAuthenticatorUsesTypedSubjectKey(t *testing.T) {
	authenticator := authrealtime.From(coreauth.RequestAuthenticatorFunc(func(context.Context, *http.Request) (*coreauth.Identity, error) {
		return &coreauth.Identity{
			Subject:     "550e8400-e29b-41d4-a716-446655440000",
			SubjectType: "user",
		}, nil
	}))

	result, err := authenticator.Authenticate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if result.UserID != "user:550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("UserID = %q", result.UserID)
	}
}

func TestRealtimeAuthenticatorKeepsUntypedSubjectCompatible(t *testing.T) {
	authenticator := authrealtime.From(coreauth.RequestAuthenticatorFunc(func(context.Context, *http.Request) (*coreauth.Identity, error) {
		return &coreauth.Identity{SubjectID: 1001}, nil
	}))

	result, err := authenticator.Authenticate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if result.UserID != "1001" {
		t.Fatalf("UserID = %q", result.UserID)
	}
}
