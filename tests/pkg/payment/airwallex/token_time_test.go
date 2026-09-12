//go:build sdkit_payment_airwallex

package airwallex_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
)

func TestAuthenticationExpiryFormats(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{"2026-01-02T04:04:05Z", true},
		{"2026-01-02T04:04:05+0000", true},
		{"2026-01-02T12:04:05.123+0800", true},
		{"2026-01-02T12:04:05+08:00", true},
		{"2026-01-02T03:04:05+0000", false},
		{"not-a-time", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			authCalls, queries := 0, 0
			c := newClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/api/v1/authentication/login" {
					authCalls++
					return reply(201, fmt.Sprintf(`{"token":"fixture-token","expires_at":%q}`, tc.value)), nil
				}
				queries++
				return reply(200, intentJSON("SUCCEEDED", "12.34", "USD")), nil
			})
			_, err := c.QueryPayment(context.Background(), queryRequest())
			if !tc.valid {
				if !errors.Is(err, payment.ErrInvalidRequest) || queries != 0 {
					t.Fatal("invalid expiry must prevent payment request")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = c.QueryPayment(context.Background(), queryRequest()); err != nil {
				t.Fatal(err)
			}
			if authCalls != 1 || queries != 2 {
				t.Fatalf("cache not reused: auth=%d query=%d", authCalls, queries)
			}
		})
	}
}
