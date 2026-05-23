package tests

import (
	"context"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/tracing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := tracing.DefaultConfig()
	if cfg.Enabled {
		t.Fatal("tracing should be disabled by default")
	}
	if cfg.ServiceName == "" {
		t.Fatal("service name should not be empty")
	}
	if cfg.Endpoint != "127.0.0.1:4317" {
		t.Fatalf("endpoint: want 127.0.0.1:4317, got %s", cfg.Endpoint)
	}
	if cfg.SampleRatio != 1 {
		t.Fatalf("sample ratio: want 1, got %f", cfg.SampleRatio)
	}
	if cfg.Timeout != 5*time.Second {
		t.Fatalf("timeout: want 5s, got %s", cfg.Timeout)
	}
}

func TestInitDisabled(t *testing.T) {
	shutdown, err := tracing.Init(context.Background(), tracing.Config{Enabled: false})
	if err != nil {
		t.Fatalf("init disabled: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown should not be nil")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown noop: %v", err)
	}
}
