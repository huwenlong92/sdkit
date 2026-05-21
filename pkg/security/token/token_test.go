package token

import "testing"

func TestTokenNonceVerifyCode(t *testing.T) {
	tok, err := RandomToken(16)
	if err != nil || tok == "" {
		t.Fatalf("token=%q err=%v", tok, err)
	}
	nonce, err := Nonce()
	if err != nil || nonce == "" {
		t.Fatalf("nonce=%q err=%v", nonce, err)
	}
	code, err := VerifyCode(6)
	if err != nil {
		t.Fatalf("code: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("code length=%d", len(code))
	}
}
