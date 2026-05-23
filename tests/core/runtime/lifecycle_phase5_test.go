package tests

import (
	"context"
	"errors"
	"os"
	"reflect"
	"syscall"
	"testing"
	"time"

	coreruntime "github.com/huwenlong92/sdkit/core/runtime"
)

func TestPhase5StopStopsProvidersThenCapabilitiesInReverseOrder(t *testing.T) {
	app := coreruntime.New()
	var calls []string
	if err := app.Use(
		testCapability{
			name:     "logger",
			register: func(*coreruntime.App) error { return nil },
			shutdown: func(context.Context) error {
				calls = append(calls, "logger.shutdown")
				return nil
			},
		},
		testCapability{
			name:     "redis",
			register: func(*coreruntime.App) error { return nil },
			shutdown: func(context.Context) error {
				calls = append(calls, "redis.shutdown")
				return nil
			},
		},
	); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Register(
		testProvider{
			name: "api",
			stop: func(context.Context) error {
				calls = append(calls, "api.stop")
				return nil
			},
		},
		testProvider{
			name: "worker",
			stop: func(context.Context) error {
				calls = append(calls, "worker.stop")
				return nil
			},
		},
	); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	want := []string{
		"worker.stop",
		"api.stop",
		"redis.shutdown",
		"logger.shutdown",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestPhase5StopCancelsRuntimeContext(t *testing.T) {
	app := coreruntime.New()
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	ctx := app.Context()
	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("app context was not canceled after Stop()")
	}
}

func TestPhase5StopTimeoutReturnsDeadlineExceeded(t *testing.T) {
	app := coreruntime.New()
	if err := app.Register(testProvider{
		name: "slow",
		stop: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := app.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop() error = %v, want deadline exceeded", err)
	}
}

func TestPhase5SignalStopsApp(t *testing.T) {
	app := coreruntime.New()
	var stopped bool
	if err := app.Register(testProvider{
		name: "api",
		stop: func(context.Context) error {
			stopped = true
			return nil
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	signals := make(chan os.Signal, 1)
	errCh := coreruntime.StopOnSignal(context.Background(), app, signals)
	signals <- syscall.SIGINT

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("StopOnSignal() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("StopOnSignal() timed out")
	}
	if !stopped {
		t.Fatal("provider was not stopped after signal")
	}
	select {
	case <-app.Context().Done():
	default:
		t.Fatal("app context was not canceled after signal")
	}
}
