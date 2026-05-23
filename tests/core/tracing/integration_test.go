package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/tracing"
)

func TestIntegrationExportsSpanToJaeger(t *testing.T) {
	if os.Getenv("SDKITGO_TRACING_INTEGRATION") != "1" {
		t.Skip("set SDKITGO_TRACING_INTEGRATION=1 to run")
	}

	endpoint := os.Getenv("SDKITGO_TRACING_ENDPOINT")
	if endpoint == "" {
		endpoint = "192.168.1.126:4317"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdown, err := tracing.Init(ctx, tracing.Config{
		Enabled:     true,
		ServiceName: "tracing-integration-test",
		Environment: "dev",
		Endpoint:    endpoint,
		Insecure:    true,
		SampleRatio: 1,
		Strict:      true,
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("init tracing: %v", err)
	}

	spanCtx, span := tracing.StartSpan(context.Background(), "integration.jaeger")
	if tracing.TraceID(spanCtx) == "" {
		t.Fatal("trace_id should not be empty")
	}
	span.End()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown tracing: %v", err)
	}
}
