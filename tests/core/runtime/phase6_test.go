package tests

import (
	"context"
	"reflect"
	"testing"

	coreruntime "github.com/huwenlong92/sdkit/core/runtime"
)

func TestPhase6RunAllProvidersOwnsShutdown(t *testing.T) {
	app := coreruntime.New()
	var calls []string
	if err := app.Use(testCapability{
		name:     "logger",
		register: func(*coreruntime.App) error { calls = append(calls, "logger.register"); return nil },
		shutdown: func(context.Context) error {
			calls = append(calls, "logger.shutdown")
			return nil
		},
	}); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Register(
		testProvider{
			name:  "api",
			start: func(context.Context) error { calls = append(calls, "api.start"); return nil },
			stop:  func(context.Context) error { calls = append(calls, "api.stop"); return nil },
		},
		testProvider{
			name:  "worker",
			start: func(context.Context) error { calls = append(calls, "worker.start"); return nil },
			stop:  func(context.Context) error { calls = append(calls, "worker.stop"); return nil },
		},
	); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := coreruntime.RunAllProviders(context.Background(), app); err != nil {
		t.Fatalf("RunAllProviders() error = %v", err)
	}

	want := []string{
		"logger.register",
		"api.start",
		"worker.start",
		"worker.stop",
		"api.stop",
		"logger.shutdown",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestPhase6RunProviderOwnsShutdownForTargetOnly(t *testing.T) {
	app := coreruntime.New()
	var calls []string
	if err := app.Register(
		testProvider{
			name:  "api",
			start: func(context.Context) error { calls = append(calls, "api.start"); return nil },
			stop:  func(context.Context) error { calls = append(calls, "api.stop"); return nil },
		},
		testProvider{
			name:  "worker",
			start: func(context.Context) error { calls = append(calls, "worker.start"); return nil },
			stop:  func(context.Context) error { calls = append(calls, "worker.stop"); return nil },
		},
	); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := coreruntime.RunProvider(context.Background(), app, "api"); err != nil {
		t.Fatalf("RunProvider(api) error = %v", err)
	}

	want := []string{"api.start", "api.stop"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}
