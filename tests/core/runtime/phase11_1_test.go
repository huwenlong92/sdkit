package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestPhase111ServiceModeRunsUntilRuntimeStop(t *testing.T) {
	app := runtime.New()
	started := make(chan struct{})
	stopped := make(chan struct{})
	runErr := make(chan error, 1)

	if err := app.Register(testProvider{
		name:     "api",
		metadata: runtime.ProviderMetadata{Mode: runtime.ProviderModeService},
		start: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return nil
		},
		stop: func(context.Context) error {
			close(stopped)
			return nil
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	go func() {
		runErr <- app.Run(context.Background())
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("service provider did not start")
	}
	waitProviderStatus(t, app, "api", runtime.StatusRunning)

	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("service provider was not stopped")
	}
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not return after Stop()")
	}
}

func TestPhase111ServiceModeReturningEarlyFails(t *testing.T) {
	app := runtime.New()
	if err := app.Register(testProvider{
		name:     "api",
		metadata: runtime.ProviderMetadata{Mode: runtime.ProviderModeService},
		start:    func(context.Context) error { return nil },
		stop:     func(context.Context) error { return nil },
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.Run(context.Background()); !errors.Is(err, runtime.ErrProviderServiceExited) {
		t.Fatalf("Run() error = %v, want ErrProviderServiceExited", err)
	}
	health := app.ProviderStatus("api")
	if health.Status != runtime.StatusFailed {
		t.Fatalf("ProviderStatus(api) = %s, want failed", health.Status)
	}
	if !errors.Is(health.Error, runtime.ErrProviderServiceExited) {
		t.Fatalf("ProviderStatus(api).Error = %v, want ErrProviderServiceExited", health.Error)
	}
}

func TestPhase111JobModeMayComplete(t *testing.T) {
	app := runtime.New()
	if err := app.Register(testProvider{
		name:     "migration",
		metadata: runtime.ProviderMetadata{Mode: runtime.ProviderModeJob},
		start:    func(context.Context) error { return nil },
		stop:     func(context.Context) error { return nil },
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	provider, ok := app.Provider("migration")
	if !ok {
		t.Fatal("Provider(migration) not found")
	}
	if mode := runtime.ProviderModeOf(provider); mode != runtime.ProviderModeJob {
		t.Fatalf("ProviderModeOf(migration) = %s, want job", mode)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func waitProviderStatus(t *testing.T, app *runtime.App, name string, want runtime.Status) {
	t.Helper()
	deadline := time.After(time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if got := app.ProviderStatus(name).Status; got == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("ProviderStatus(%s) did not become %s; got %s", name, want, app.ProviderStatus(name).Status)
		case <-tick.C:
		}
	}
}
