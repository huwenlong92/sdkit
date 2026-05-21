package hashid

import (
	"errors"
	"testing"
)

func TestEncoderTyped(t *testing.T) {
	encoder, err := New(Config{Salt: "test-salt", Alphabet: LowerAlphabet, MinLength: 8})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	raw, err := encoder.EncodeTyped(12345, 100)
	if err != nil {
		t.Fatalf("EncodeTyped() error = %v", err)
	}
	got, err := encoder.DecodeTyped(raw, 100)
	if err != nil {
		t.Fatalf("DecodeTyped() error = %v", err)
	}
	if got != 12345 {
		t.Fatalf("DecodeTyped() = %d, want 12345", got)
	}
}

func TestEncoderTypeMismatch(t *testing.T) {
	encoder, err := New(Config{Salt: "test-salt"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	raw, err := encoder.EncodeTyped(1, 10)
	if err != nil {
		t.Fatalf("EncodeTyped() error = %v", err)
	}
	_, err = encoder.DecodeTyped(raw, 11)
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("DecodeTyped() error = %v, want ErrTypeMismatch", err)
	}
}

func TestNewRequiresSalt(t *testing.T) {
	_, err := New(Config{})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("New() error = %v, want ErrInvalidConfig", err)
	}
}
