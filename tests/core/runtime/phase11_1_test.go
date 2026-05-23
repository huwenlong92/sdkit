package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	coreruntime "github.com/huwenlong92/sdkit/core/runtime"
)

func TestPhase111ServiceModeRunsUntilRuntimeStop(t *testing.T) {
	app := coreruntime.New()
	started := make(chan struct{})
	stopped := make(chan struct{})
	runErr := make(chan error, 1)

	if err := app.Register(testProvider{
		name:     "api",
		metadata: coreruntime.ProviderMetadata{Mode: coreruntime.ProviderModeService},
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
	waitProviderStatus(t, app, "api", coreruntime.StatusRunning)

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
	app := coreruntime.New()
	if err := app.Register(testProvider{
		name:     "api",
		metadata: coreruntime.ProviderMetadata{Mode: coreruntime.ProviderModeService},
		start:    func(context.Context) error { return nil },
		stop:     func(context.Context) error { return nil },
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.Run(context.Background()); !errors.Is(err, coreruntime.ErrProviderServiceExited) {
		t.Fatalf("Run() error = %v, want ErrProviderServiceExited", err)
	}
	health := app.ProviderStatus("api")
	if health.Status != coreruntime.StatusFailed {
		t.Fatalf("ProviderStatus(api) = %s, want failed", health.Status)
	}
	if !errors.Is(health.Error, coreruntime.ErrProviderServiceExited) {
		t.Fatalf("ProviderStatus(api).Error = %v, want ErrProviderServiceExited", health.Error)
	}
}

func TestPhase111JobModeMayComplete(t *testing.T) {
	app := coreruntime.New()
	if err := app.Register(testProvider{
		name:     "migration",
		metadata: coreruntime.ProviderMetadata{Mode: coreruntime.ProviderModeJob},
		start:    func(context.Context) error { return nil },
		stop:     func(context.Context) error { return nil },
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	provider, ok := app.Provider("migration")
	if !ok {
		t.Fatal("Provider(migration) not found")
	}
	if mode := coreruntime.ProviderModeOf(provider); mode != coreruntime.ProviderModeJob {
		t.Fatalf("ProviderModeOf(migration) = %s, want job", mode)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func waitProviderStatus(t *testing.T, app *coreruntime.App, name string, want coreruntime.Status) {
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
