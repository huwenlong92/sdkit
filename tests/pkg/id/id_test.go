package id_test

import (
	"strings"
	"testing"

	"github.com/huwenlong92/sdkit/pkg/id"
	"github.com/segmentio/ksuid"
)

func TestNewReturnsKSUID(t *testing.T) {
	value, err := id.New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := ksuid.Parse(value); err != nil {
		t.Fatalf("New() = %q, want valid KSUID: %v", value, err)
	}
}

func TestNewPrefixedReturnsPrefixedKSUID(t *testing.T) {
	const prefix = "job_"

	value, err := id.NewPrefixed(prefix)
	if err != nil {
		t.Fatalf("NewPrefixed() error = %v", err)
	}
	if !strings.HasPrefix(value, prefix) {
		t.Fatalf("NewPrefixed() = %q, want prefix %q", value, prefix)
	}
	if _, err := ksuid.Parse(strings.TrimPrefix(value, prefix)); err != nil {
		t.Fatalf("NewPrefixed() = %q, want valid KSUID suffix: %v", value, err)
	}
}
