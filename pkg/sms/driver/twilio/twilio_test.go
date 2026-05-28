//go:build sdkit_sms_twilio

package twilio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/huwenlong92/sdkit/pkg/sms"
)

func TestProviderSendsContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Accounts/AC123/Messages.json" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "AC123" || password != "token" {
			t.Fatalf("unexpected auth: %q %q %v", username, password, ok)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.Form.Get("To") != "+15558675310" {
			t.Fatalf("to = %q", r.Form.Get("To"))
		}
		if r.Form.Get("From") != "+15557122661" {
			t.Fatalf("from = %q", r.Form.Get("From"))
		}
		if r.Form.Get("Body") != "hello" {
			t.Fatalf("body = %q", r.Form.Get("Body"))
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sid":    "SM123",
			"status": "queued",
			"to":     r.Form.Get("To"),
			"from":   r.Form.Get("From"),
			"body":   r.Form.Get("Body"),
		})
	}))
	defer server.Close()

	provider, err := New("twilio_global", sms.ProviderConfig{
		Driver:   "twilio",
		Account:  "AC123",
		Password: "token",
		Sender:   "+15557122661",
		Endpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	result, err := provider.Send(context.Background(), sms.ProviderRequest{
		To: []string{"+15558675310"},
		Payload: sms.Payload{
			Content: "hello",
		},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !result.Success {
		t.Fatalf("success = false")
	}
	if result.Provider != "twilio_global" {
		t.Fatalf("provider = %q", result.Provider)
	}
	if result.MessageNo != "SM123" {
		t.Fatalf("message no = %q", result.MessageNo)
	}
	if result.Code != "queued" {
		t.Fatalf("code = %q", result.Code)
	}
}
