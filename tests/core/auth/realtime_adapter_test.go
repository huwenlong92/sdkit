package auth_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/huwenlong92/sdkit/core/auth"
	"github.com/huwenlong92/sdkit/core/auth/adapter/realtime"
)

func TestRealtimeAuthenticatorUsesTypedSubjectKey(t *testing.T) {
	authenticator := realtime.From(auth.RequestAuthenticatorFunc(func(context.Context, *http.Request) (*auth.Identity, error) {
		return &auth.Identity{
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
	authenticator := realtime.From(auth.RequestAuthenticatorFunc(func(context.Context, *http.Request) (*auth.Identity, error) {
		return &auth.Identity{SubjectID: 1001}, nil
	}))

	result, err := authenticator.Authenticate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if result.UserID != "1001" {
		t.Fatalf("UserID = %q", result.UserID)
	}
}
