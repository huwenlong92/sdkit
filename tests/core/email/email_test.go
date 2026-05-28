package email_test

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/huwenlong92/sdkit/core/email"
)

type fakeEmailProvider struct {
	name string
	fail bool
}

func (p *fakeEmailProvider) Send(_ context.Context, payload email.Payload) (*email.ProviderResult, error) {
	if p.fail {
		return nil, errors.New("send failed")
	}
	return &email.ProviderResult{MessageID: p.name + "-message", Raw: payload}, nil
}

func (p *fakeEmailProvider) Close() error {
	return nil
}

func init() {
	email.RegisterDriver("test_email", func(name string, cfg email.ProviderConfig) (email.Provider, error) {
		return &fakeEmailProvider{name: name, fail: cfg.Host == "fail"}, nil
	})
}

func TestManagerFallsBackToNextEmailProvider(t *testing.T) {
	manager, err := email.NewManager(email.Config{
		Default:  "primary",
		Fallback: []string{"backup"},
		Providers: map[string]email.ProviderConfig{
			"primary": {Driver: "test_email", Host: "fail"},
			"backup":  {Driver: "test_email"},
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	result, err := manager.Send(context.Background(), email.DirectMessage{
		To:      []string{"user@example.com"},
		Subject: "hello",
		Text:    "hello",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.Provider != "backup" {
		t.Fatalf("provider = %q, want backup", result.Provider)
	}
	if result.Result == nil || result.Result.MessageID != "backup-message" {
		t.Fatalf("result = %+v, want backup-message", result.Result)
	}
	if len(result.Attempts) != 2 || result.Attempts[0].Success || !result.Attempts[1].Success {
		t.Fatalf("unexpected attempts: %+v", result.Attempts)
	}
}

func TestManagerReturnsFailedSendResult(t *testing.T) {
	manager, err := email.NewManager(email.Config{
		Default: "primary",
		Providers: map[string]email.ProviderConfig{
			"primary": {Driver: "test_email", Host: "fail"},
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	result, err := manager.Send(context.Background(), email.DirectMessage{
		To:      []string{"user@example.com"},
		Subject: "hello",
		Text:    "hello",
	})
	var noProvider *email.NoProviderAvailableError
	if !errors.As(err, &noProvider) {
		t.Fatalf("expected no provider available, got %v", err)
	}
	if result == nil || result.Error == nil {
		t.Fatalf("expected failed send result, got %+v", result)
	}
	if len(result.Attempts) != 1 || result.Attempts[0].Provider != "primary" {
		t.Fatalf("unexpected result attempts: %+v", result.Attempts)
	}
}

func TestEmailMiddlewareCanBlockSend(t *testing.T) {
	manager, err := email.NewManager(email.Config{
		Default: "primary",
		Providers: map[string]email.ProviderConfig{
			"primary": {Driver: "test_email"},
		},
	}, func(next email.Sender) email.Sender {
		return email.SenderFunc(func(context.Context, email.Request) (*email.SendResult, error) {
			return nil, errors.New("blocked")
		})
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	_, err = manager.Send(context.Background(), email.DirectMessage{To: []string{"user@example.com"}})
	if err == nil || err.Error() != "blocked" {
		t.Fatalf("expected blocked error, got %v", err)
	}
}

func TestTemplateMessageRendersConfiguredTemplate(t *testing.T) {
	manager, err := email.NewManager(email.Config{
		Default: "primary",
		Providers: map[string]email.ProviderConfig{
			"primary": {Driver: "test_email"},
		},
		Templates: map[string]email.Template{
			"verify_code": {
				Subject: "验证码 {{.code}}",
				Text:    "您的验证码是 {{.code}}",
				HTML:    "<p>您的验证码是 <strong>{{.code}}</strong></p>",
			},
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	result, err := manager.Send(context.Background(), email.TemplateMessage{
		To:       []string{"user@example.com"},
		Template: "verify_code",
		Data: map[string]any{
			"code": "123456",
		},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	payload, ok := result.Result.Raw.(email.Payload)
	if !ok {
		t.Fatalf("raw payload type = %T", result.Result.Raw)
	}
	if payload.Subject != "验证码 123456" {
		t.Fatalf("subject = %q", payload.Subject)
	}
	if payload.Text != "您的验证码是 123456" {
		t.Fatalf("text = %q", payload.Text)
	}
	if payload.HTML != "<p>您的验证码是 <strong>123456</strong></p>" {
		t.Fatalf("html = %q", payload.HTML)
	}
}

func TestLoadTemplatesLoadsTemplateFiles(t *testing.T) {
	templates, err := email.LoadTemplates(fstest.MapFS{
		"verify_code.txt":  {Data: []byte("您的验证码是 {{.code}}")},
		"verify_code.html": {Data: []byte("<p>您的验证码是 <strong>{{.code}}</strong></p>")},
	}, map[string]email.Template{
		"verify_code": {
			Subject:  "验证码 {{.code}}",
			TextFile: "verify_code.txt",
			HTMLFile: "verify_code.html",
		},
	})
	if err != nil {
		t.Fatalf("load templates: %v", err)
	}

	manager, err := email.NewManager(email.Config{
		Default: "primary",
		Providers: map[string]email.ProviderConfig{
			"primary": {Driver: "test_email"},
		},
		Templates: templates,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	result, err := manager.Send(context.Background(), email.TemplateMessage{
		To:       []string{"user@example.com"},
		Template: "verify_code",
		Data:     map[string]any{"code": "123456"},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	payload := result.Result.Raw.(email.Payload)
	if payload.Text != "您的验证码是 123456" {
		t.Fatalf("text = %q", payload.Text)
	}
	if payload.HTML != "<p>您的验证码是 <strong>123456</strong></p>" {
		t.Fatalf("html = %q", payload.HTML)
	}
}
