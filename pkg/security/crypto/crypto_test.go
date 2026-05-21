package crypto

import "testing"

func TestAESGCMAndSignature(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	plain := []byte("hello")
	ciphertext, err := EncryptAESGCM(key, plain, nil)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	got, err := DecryptAESGCM(key, ciphertext, nil)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("decrypt got %q", got)
	}
	sig := SignHMACSHA256([]byte("secret"), "post", "/v1", "1", "n", []byte("{}"))
	if sig == "" || !EqualHMACHex(sig, sig) {
		t.Fatalf("invalid signature")
	}
}
