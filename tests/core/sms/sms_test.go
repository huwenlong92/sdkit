package sms_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/sms"
)

type fakeSMSProvider struct {
	name string
	fail bool
}

func (p *fakeSMSProvider) Send(context.Context, sms.ProviderRequest) (*sms.ProviderResult, error) {
	if p.fail {
		return &sms.ProviderResult{Provider: p.name, Success: false, Message: "send failed"}, nil
	}
	return &sms.ProviderResult{Provider: p.name, Success: true, MessageNo: p.name + "-message"}, nil
}

func (p *fakeSMSProvider) Close() error {
	return nil
}

type fakeLimiter struct {
	allow bool
}

func (l fakeLimiter) Allow(context.Context, string, int64, time.Duration) (bool, error) {
	return l.allow, nil
}

func init() {
	sms.RegisterDriver("test_sms", func(name string, cfg sms.ProviderConfig) (sms.Provider, error) {
		return &fakeSMSProvider{name: name, fail: cfg.Account == "fail"}, nil
	})
}

func TestSMSDefaultProviderDoesNotFallbackImplicitly(t *testing.T) {
	manager, err := sms.NewManager(sms.Config{
		Default: "primary",
		Providers: map[string]sms.ProviderConfig{
			"primary": {Driver: "test_sms", Account: "fail"},
			"backup":  {Driver: "test_sms"},
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	_, err = manager.Send(context.Background(), []string{"13800138000"}, sms.TemplateMessage{
		Content: "hello",
	})
	var noProvider *sms.NoProviderAvailableError
	if !errors.As(err, &noProvider) {
		t.Fatalf("expected no provider available, got %v", err)
	}
	if len(noProvider.Attempts) != 1 || noProvider.Attempts[0].Provider != "primary" {
		t.Fatalf("unexpected attempts: %+v", noProvider.Attempts)
	}
}

func TestSMSMessageProvidersEnableFallback(t *testing.T) {
	manager, err := sms.NewManager(sms.Config{
		Default: "primary",
		Providers: map[string]sms.ProviderConfig{
			"primary": {Driver: "test_sms", Account: "fail"},
			"backup":  {Driver: "test_sms"},
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	result, err := manager.Send(context.Background(), []string{"13800138000"}, sms.TemplateMessage{
		Content:       "hello",
		ProviderNames: []string{"primary", "backup"},
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

func TestSMSRateLimitMiddlewareCanBlockSend(t *testing.T) {
	manager, err := sms.NewManager(sms.Config{
		Default: "primary",
		Providers: map[string]sms.ProviderConfig{
			"primary": {Driver: "test_sms"},
		},
	}, sms.RateLimitMiddleware(fakeLimiter{allow: false}, sms.RateLimitRule{
		Limit:  1,
		Window: time.Minute,
	}))
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	_, err = manager.Send(context.Background(), []string{"13800138000"}, sms.TemplateMessage{Content: "hello"})
	if !errors.Is(err, sms.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}
