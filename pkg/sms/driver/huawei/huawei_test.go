//go:build sdkit_sms_huawei

package huawei

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/huwenlong92/sdkit/pkg/sms"
)

func TestProviderSendsTemplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != sendPath {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != `WSSE realm="SDP",profile="UsernameToken",type="Appkey"` {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if !strings.Contains(r.Header.Get("X-WSSE"), `Username="app-key"`) {
			t.Fatalf("x-wsse = %q", r.Header.Get("X-WSSE"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.Form.Get("from") != "csms100000001" {
			t.Fatalf("from = %q", r.Form.Get("from"))
		}
		if r.Form.Get("to") != "+8613800138000" {
			t.Fatalf("to = %q", r.Form.Get("to"))
		}
		if r.Form.Get("templateId") != "TPL001" {
			t.Fatalf("templateId = %q", r.Form.Get("templateId"))
		}
		if r.Form.Get("templateParas") != `["123456"]` {
			t.Fatalf("templateParas = %q", r.Form.Get("templateParas"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":        "000000",
			"description": "Success",
			"result": []map[string]any{
				{
					"smsMsgId": "huawei-message",
					"from":     "csms100000001",
					"originTo": "+8613800138000",
					"status":   "000000",
				},
			},
		})
	}))
	defer server.Close()

	provider, err := New("huawei_cn", sms.ProviderConfig{
		AppKey:    "app-key",
		AppSecret: "app-secret",
		Sender:    "csms100000001",
		SignName:  "华为云",
		Endpoint:  server.URL,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	result, err := provider.Send(context.Background(), sms.ProviderRequest{
		To: []string{"+8613800138000"},
		Payload: sms.Payload{
			Template: "TPL001",
			Data:     []sms.Param{{Key: "code", Value: "123456"}},
		},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !result.Success {
		t.Fatalf("success = false")
	}
	if result.MessageNo != "huawei-message" {
		t.Fatalf("message no = %q", result.MessageNo)
	}
}
