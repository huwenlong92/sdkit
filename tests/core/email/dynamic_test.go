package email_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/huwenlong92/sdkit/core/email"
)

var dynamicProviderCloseCount atomic.Int64

type dynamicEmailProvider struct {
	name string
}

func (p *dynamicEmailProvider) Send(_ context.Context, payload email.Payload) (*email.ProviderResult, error) {
	return &email.ProviderResult{MessageID: p.name + "-message", Raw: payload}, nil
}

func (p *dynamicEmailProvider) Close() error {
	dynamicProviderCloseCount.Add(1)
	return nil
}

func init() {
	email.RegisterDriver("dynamic_test_email", func(name string, _ email.ProviderConfig) (email.Provider, error) {
		return &dynamicEmailProvider{name: name}, nil
	})
}

func TestDynamicManagerResolvesProviderForEverySend(t *testing.T) {
	dynamicProviderCloseCount.Store(0)
	var resolveCount atomic.Int64
	manager, err := email.NewDynamicManager(email.ProviderResolverFunc(func(ctx context.Context, name string) (email.ProviderConfig, error) {
		if err := ctx.Err(); err != nil {
			return email.ProviderConfig{}, err
		}
		if name != "account_a" {
			return email.ProviderConfig{}, errors.New("unknown account")
		}
		resolveCount.Add(1)
		return email.ProviderConfig{Driver: "dynamic_test_email"}, nil
	}), nil)
	if err != nil {
		t.Fatalf("new dynamic manager: %v", err)
	}

	for range 2 {
		result, sendErr := manager.SendVia(context.Background(), email.DirectMessage{
			To:      []string{"user@example.com"},
			Subject: "hello",
			Text:    "hello",
		}, "account_a")
		if sendErr != nil {
			t.Fatalf("send via dynamic account: %v", sendErr)
		}
		if result.Provider != "account_a" {
			t.Fatalf("provider = %q, want account_a", result.Provider)
		}
	}

	if got := resolveCount.Load(); got != 2 {
		t.Fatalf("resolver calls = %d, want 2", got)
	}
	if got := dynamicProviderCloseCount.Load(); got != 2 {
		t.Fatalf("provider close calls = %d, want 2", got)
	}
}

func TestDynamicManagerRequiresExplicitRoute(t *testing.T) {
	manager, err := email.NewDynamicManager(email.ProviderResolverFunc(func(context.Context, string) (email.ProviderConfig, error) {
		return email.ProviderConfig{Driver: "dynamic_test_email"}, nil
	}), nil)
	if err != nil {
		t.Fatalf("new dynamic manager: %v", err)
	}

	_, err = manager.Send(context.Background(), email.DirectMessage{To: []string{"user@example.com"}})
	if !errors.Is(err, email.ErrDefaultRequired) {
		t.Fatalf("send error = %v, want %v", err, email.ErrDefaultRequired)
	}
	_, err = manager.SendVia(context.Background(), email.DirectMessage{To: []string{"user@example.com"}})
	if !errors.Is(err, email.ErrDefaultRequired) {
		t.Fatalf("send via error = %v, want %v", err, email.ErrDefaultRequired)
	}
}
