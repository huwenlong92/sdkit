//go:build sdkit_sms_aliyun_global

package sms_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/huwenlong92/sdkit/pkg/sms"
	aliyunglobal "github.com/huwenlong92/sdkit/pkg/sms/driver/aliyun_global"
)

func TestAliyunGlobalProviderSendsContent(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		want := map[string]string{
			"Action":   "SendMessageToGlobe",
			"Version":  "2018-05-01",
			"RegionId": "ap-southeast-1",
			"To":       "14155552671",
			"Message":  "Your verification code is 123456.",
			"From":     "EJoy",
		}
		for key, value := range want {
			if request.Form.Get(key) != value {
				t.Errorf("form %s = %q, want %q", key, request.Form.Get(key), value)
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"ResponseCode":        "OK",
			"ResponseDescription": "The SMS Send Request was accepted",
			"MessageId":           "1008030300123456",
		})
	}))
	defer server.Close()

	provider, err := aliyunglobal.New("aliyun_global_main", sms.ProviderConfig{
		AccessKeyID:     "access-key-id",
		AccessKeySecret: "access-key-secret",
		RegionID:        "ap-southeast-1",
		Endpoint:        server.URL,
		Sender:          "EJoy",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	result, err := provider.Send(context.Background(), sms.ProviderRequest{
		To:      []string{"14155552671"},
		Payload: sms.Payload{Content: "Your verification code is 123456."},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !result.Success || result.Code != "OK" || result.MessageNo != "1008030300123456" {
		t.Fatalf("result = %+v", result)
	}
}

func TestAliyunGlobalProviderValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		config sms.ProviderConfig
	}{
		{name: "missing access key", config: sms.ProviderConfig{AccessKeySecret: "secret"}},
		{name: "missing access secret", config: sms.ProviderConfig{AccessKeyID: "key"}},
		{name: "endpoint path", config: sms.ProviderConfig{AccessKeyID: "key", AccessKeySecret: "secret", Endpoint: "https://example.com/path"}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := aliyunglobal.New("test", test.config); err == nil {
				t.Fatal("New error = nil")
			}
		})
	}
}
