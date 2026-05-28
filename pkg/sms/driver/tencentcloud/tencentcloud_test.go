//go:build sdkit_sms_tencentcloud

package tencentcloud

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
		if r.URL.Path != "/" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("X-TC-Action") != "SendSms" {
			t.Fatalf("action = %q", r.Header.Get("X-TC-Action"))
		}
		if r.Header.Get("X-TC-Version") != version {
			t.Fatalf("version = %q", r.Header.Get("X-TC-Version"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), algorithm+" Credential=secret-id/") {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		var body sendSMSRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.SmsSdkAppID != "1400006666" {
			t.Fatalf("app id = %q", body.SmsSdkAppID)
		}
		if body.TemplateID != "1110" {
			t.Fatalf("template id = %q", body.TemplateID)
		}
		if len(body.PhoneNumberSet) != 1 || body.PhoneNumberSet[0] != "+8613800138000" {
			t.Fatalf("phones = %+v", body.PhoneNumberSet)
		}
		if len(body.TemplateParamSet) != 1 || body.TemplateParamSet[0] != "123456" {
			t.Fatalf("params = %+v", body.TemplateParamSet)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Response": map[string]any{
				"SendStatusSet": []map[string]any{
					{
						"SerialNo":    "tencent-message",
						"PhoneNumber": "+8613800138000",
						"Code":        "Ok",
						"Message":     "send success",
					},
				},
				"RequestId": "request-id",
			},
		})
	}))
	defer server.Close()

	provider, err := New("tencent_cn", sms.ProviderConfig{
		AccessKeyID:     "secret-id",
		AccessKeySecret: "secret-key",
		SmsSdkAppID:     "1400006666",
		SignName:        "腾讯云",
		Endpoint:        server.URL,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	result, err := provider.Send(context.Background(), sms.ProviderRequest{
		To: []string{"+8613800138000"},
		Payload: sms.Payload{
			Template: "1110",
			Data:     []sms.Param{{Key: "code", Value: "123456"}},
		},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !result.Success {
		t.Fatalf("success = false")
	}
	if result.MessageNo != "tencent-message" {
		t.Fatalf("message no = %q", result.MessageNo)
	}
}
