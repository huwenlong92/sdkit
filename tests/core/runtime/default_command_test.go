package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	coreruntime "github.com/huwenlong92/sdkit/core/runtime"
)

type managedCommandProvider struct {
	testProvider
}

func (p managedCommandProvider) RuntimeManaged() bool {
	return true
}

func TestDefaultServeCommandMetadataAndArgs(t *testing.T) {
	cmd := coreruntime.NewServeCommand()
	if cmd.Name() != "serve" {
		t.Fatalf("Name() = %s, want serve", cmd.Name())
	}
	if metadata := cmd.Metadata(); metadata.Name != "serve" || metadata.Group != coreruntime.GroupSystem {
		t.Fatalf("Metadata() = %+v, want serve system command", metadata)
	}
	if err := cmd.Run(context.Background(), coreruntime.New(), []string{"api"}); err == nil {
		t.Fatal("Run() with args must return error")
	}
}

func TestDefaultRunCommandMetadataAndArgs(t *testing.T) {
	cmd := coreruntime.NewRunCommand()
	if cmd.Name() != "run" {
		t.Fatalf("Name() = %s, want run", cmd.Name())
	}
	if metadata := cmd.Metadata(); metadata.Name != "run" || metadata.Group != coreruntime.GroupSystem {
		t.Fatalf("Metadata() = %+v, want run system command", metadata)
	}
	if err := cmd.Run(context.Background(), coreruntime.New(), nil); err == nil {
		t.Fatal("Run() without provider name must return error")
	}
}

func TestDefaultServeCommandWaitsForManagedProviderStop(t *testing.T) {
	app := coreruntime.New()
	started := make(chan struct{})
	stopped := make(chan struct{})
	if err := app.Register(managedCommandProvider{
		testProvider: testProvider{
			name:     "api",
			metadata: coreruntime.ProviderMetadata{Mode: coreruntime.ProviderModeJob},
			start: func(context.Context) error {
				close(started)
				return nil
			},
			stop: func(context.Context) error {
				close(stopped)
				return nil
			},
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- coreruntime.NewServeCommand().Run(context.Background(), app, nil)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("managed provider did not start")
	}
	select {
	case err := <-done:
		t.Fatalf("serve command returned before app stop: %v", err)
	default:
	}

	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("managed provider did not stop")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve command returned error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serve command did not return after app stop")
	}
}

func TestDefaultRunCommandFallsBackToRuntimeRunProvider(t *testing.T) {
	app := coreruntime.New()
	if err := app.Register(testProvider{
		name: "job",
		start: func(context.Context) error {
			return errors.New("job failed")
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := coreruntime.NewRunCommand().Run(context.Background(), app, []string{"job"}); err == nil {
		t.Fatal("Run() must return provider error")
	}
}
