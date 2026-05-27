//go:build !sdkit_tracing_otel

package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/tracing"
)

func TestInitEnabledWithoutBuildTagReturnsError(t *testing.T) {
	shutdown, err := tracing.Init(context.Background(), tracing.Config{Enabled: true})
	if !errors.Is(err, tracing.ErrNotCompiled) {
		t.Fatalf("init enabled without tag: want ErrNotCompiled, got %v", err)
	}
	if shutdown != nil {
		t.Fatal("shutdown should be nil when tracing is enabled but not compiled in")
	}
	if tracing.Enabled() {
		t.Fatal("tracing should remain disabled when full tracing is not compiled in")
	}
}
