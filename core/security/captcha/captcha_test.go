package captcha

import (
	"context"
	"testing"
)

func TestMemoryProvider(t *testing.T) {
	p := NewMemoryProvider("ok")
	if err := p.Verify(context.Background(), "ok", "127.0.0.1"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := p.Verify(context.Background(), "ok", "127.0.0.1"); err == nil {
		t.Fatalf("used token should fail")
	}
}
