package email_test

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/email"
)

type fakeEmailProvider struct {
	name string
	fail bool
}

func (p *fakeEmailProvider) Send(context.Context, email.Message) (*email.ProviderResult, error) {
	if p.fail {
		return nil, errors.New("send failed")
	}
	return &email.ProviderResult{MessageID: p.name + "-message"}, nil
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

	result, err := manager.Send(context.Background(), email.Message{
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
	if len(result.Attempts) != 2 || result.Attempts[0].Success || !result.Attempts[1].Success {
		t.Fatalf("unexpected attempts: %+v", result.Attempts)
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

	_, err = manager.Send(context.Background(), email.Message{To: []string{"user@example.com"}})
	if err == nil || err.Error() != "blocked" {
		t.Fatalf("expected blocked error, got %v", err)
	}
}
