package captcha

import (
	"context"
	"testing"
)

func TestMemoryProvider(t *testing.T) {
	p := NewMemoryProvider("ok")
	if err := p.Verify(context.Background(), "", "ok"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := p.Verify(context.Background(), "", "ok"); err == nil {
		t.Fatalf("used token should fail")
	}
}

func TestManagerGenerateAndVerify(t *testing.T) {
	manager := NewManager(NewMemoryProvider())
	challenge, err := manager.Generate(context.Background(), KindImage, GenerateOptions{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	provider := manager.providers[KindImage].(*MemoryProvider)
	answer := provider.tokens[challenge.ID]
	if answer == "" {
		t.Fatalf("missing generated answer")
	}
	if err := manager.Verify(context.Background(), KindImage, challenge.ID, answer); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := manager.Verify(context.Background(), KindImage, challenge.ID, answer); err == nil {
		t.Fatalf("used challenge should fail")
	}
}

func TestBase64Provider(t *testing.T) {
	store := newTestCaptchaStore()
	provider := NewBase64Provider(WithBase64Store(store))
	challenge, err := provider.Generate(context.Background(), GenerateOptions{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if challenge.ID == "" || challenge.Image == "" || challenge.Kind != KindImage {
		t.Fatalf("bad challenge: %+v", challenge)
	}
	answer := store.values[challenge.ID]
	if answer == "" {
		t.Fatalf("missing answer in store")
	}
	if err := provider.Verify(context.Background(), challenge.ID, answer); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := provider.Verify(context.Background(), challenge.ID, answer); err == nil {
		t.Fatalf("used captcha should fail")
	}
}

type testCaptchaStore struct {
	values map[string]string
}

func newTestCaptchaStore() *testCaptchaStore {
	return &testCaptchaStore{values: make(map[string]string)}
}

func (s *testCaptchaStore) Set(id string, value string) error {
	s.values[id] = value
	return nil
}

func (s *testCaptchaStore) Get(id string, clear bool) string {
	value := s.values[id]
	if clear {
		delete(s.values, id)
	}
	return value
}

func (s *testCaptchaStore) Verify(id string, answer string, clear bool) bool {
	return s.Get(id, clear) == answer
}
