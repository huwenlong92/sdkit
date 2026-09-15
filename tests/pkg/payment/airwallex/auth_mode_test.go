//go:build sdkit_payment_airwallex

package airwallex_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/airwallex/httpapi"
)

func TestAuthenticationModeHeadersAndTokenReuse(t *testing.T) {
	tests := []struct {
		name              string
		mode              httpapi.AuthenticationMode
		wantAccountHeader string
	}{
		{name: "unchanged zero value", wantAccountHeader: "acct_fixture"},
		{name: "explicit account", mode: httpapi.AuthenticateAccount, wantAccountHeader: "acct_fixture"},
		{name: "single account scoped key", mode: httpapi.AuthenticateDefault},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authCalls := 0
			cfg := config(func(req *http.Request) (*http.Response, error) {
				if response := login(req); response != nil {
					authCalls++
					if got := req.Header.Get("x-login-as"); got != tc.wantAccountHeader {
						t.Fatalf("account header=%q, want %q", got, tc.wantAccountHeader)
					}
					if req.Header.Get("x-client-id") != "fictional-client" || req.Header.Get("x-api-key") != "fictional-api-key" {
						t.Fatal("missing fixture authentication headers")
					}
					return response, nil
				}
				if req.Header.Get("Authorization") != "Bearer fixture-access-token" || req.Header.Get("x-api-key") != "" || req.Header.Get("x-login-as") != "" {
					t.Fatal("operation must use bearer token only")
				}
				return reply(200, intentJSON("SUCCEEDED", "12.34", "USD")), nil
			})
			cfg.AuthenticationMode = tc.mode
			client, err := httpapi.NewClient(cfg)
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if _, err := client.QueryPayment(context.Background(), queryRequest()); err != nil {
					t.Fatal(err)
				}
			}
			if authCalls != 1 {
				t.Fatalf("auth calls=%d, want 1", authCalls)
			}
		})
	}
}

func TestDefaultAuthenticationPreservesWebhookAccountVerification(t *testing.T) {
	cfg := config(nil)
	cfg.AuthenticationMode = httpapi.AuthenticateDefault
	client, err := httpapi.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	notification := signed("payment_intent.succeeded", intentJSON("SUCCEEDED", "12.34", "USD"), fixedNow)
	if _, err := client.ParseNotify(context.Background(), notification); err != nil {
		t.Fatal(err)
	}
	cfg.AccountID = "acct_other"
	other, err := httpapi.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.ParseNotify(context.Background(), notification); !errors.Is(err, payment.ErrNotifyVerificationFail) {
		t.Fatalf("wrong account error=%v, want verification failure", err)
	}
	cfg.AccountID = ""
	if _, err := httpapi.NewClient(cfg); !errors.Is(err, payment.ErrInvalidRequest) {
		t.Fatalf("missing account error=%v, want invalid request", err)
	}
	cfg.AccountID = "acct_fixture"
	cfg.AuthenticationMode = "unsupported"
	if _, err := httpapi.NewClient(cfg); !errors.Is(err, payment.ErrInvalidRequest) {
		t.Fatalf("unknown mode error=%v, want invalid request", err)
	}
}
