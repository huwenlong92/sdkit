package tests

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/tracing"
)

func TestShutdownNoop(t *testing.T) {
	if err := tracing.ShutdownNoop(context.Background()); err != nil {
		t.Fatalf("shutdown noop: %v", err)
	}
}
