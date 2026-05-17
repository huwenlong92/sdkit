package password

import "testing"

func TestHashVerify(t *testing.T) {
	hash, err := Hash("abc12345")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !Verify(hash, "abc12345") {
		t.Fatalf("password should verify")
	}
	if Verify(hash, "bad") {
		t.Fatalf("bad password should not verify")
	}
}
