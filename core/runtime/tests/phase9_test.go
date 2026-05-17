package tests

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	coreruntime "github.com/huwenlong92/sdkit/core/runtime"
)

func TestPhase9ProviderLifecycleStatus(t *testing.T) {
	app := coreruntime.New()
	if err := app.Register(testProvider{
		name: "api",
		start: func(context.Context) error {
			if got := app.ProviderStatus("api").Status; got != coreruntime.StatusBooting {
				t.Fatalf("ProviderStatus(api) during Start = %s, want booting", got)
			}
			return nil
		},
		stop: func(context.Context) error {
			if got := app.ProviderStatus("api").Status; got != coreruntime.StatusStopping {
				t.Fatalf("ProviderStatus(api) during Stop = %s, want stopping", got)
			}
			return nil
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if got := app.ProviderStatus("api").Status; got != coreruntime.StatusStopped {
		t.Fatalf("ProviderStatus(api) before Run = %s, want stopped", got)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := app.ProviderStatus("api").Status; got != coreruntime.StatusRunning {
		t.Fatalf("ProviderStatus(api) after Run = %s, want running", got)
	}
	provider, ok := app.Provider("api")
	if !ok {
		t.Fatal("Provider(api) not found")
	}
	if got := provider.Status(); got != coreruntime.StatusRunning {
		t.Fatalf("Provider(api).Status() = %s, want running", got)
	}
	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if got := app.ProviderStatus("api").Status; got != coreruntime.StatusStopped {
		t.Fatalf("ProviderStatus(api) after Stop = %s, want stopped", got)
	}
}

func TestPhase9CapabilityLifecycleStatus(t *testing.T) {
	app := coreruntime.New()
	if err := app.Use(testCapability{
		name: "database",
		register: func(*coreruntime.App) error {
			if got := app.CapabilityStatus("database").Status; got != coreruntime.StatusBooting {
				t.Fatalf("CapabilityStatus(database) during Register = %s, want booting", got)
			}
			return nil
		},
		shutdown: func(context.Context) error {
			if got := app.CapabilityStatus("database").Status; got != coreruntime.StatusStopping {
				t.Fatalf("CapabilityStatus(database) during Shutdown = %s, want stopping", got)
			}
			return nil
		},
	}); err != nil {
		t.Fatalf("Use() error = %v", err)
	}

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := app.CapabilityStatus("database").Status; got != coreruntime.StatusRunning {
		t.Fatalf("CapabilityStatus(database) after Run = %s, want running", got)
	}
	capability, ok := app.Capability("database")
	if !ok {
		t.Fatal("Capability(database) not found")
	}
	if got := capability.Status(); got != coreruntime.StatusRunning {
		t.Fatalf("Capability(database).Status() = %s, want running", got)
	}
	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if got := app.CapabilityStatus("database").Status; got != coreruntime.StatusStopped {
		t.Fatalf("CapabilityStatus(database) after Stop = %s, want stopped", got)
	}
}

func TestPhase9BootFailureMarksProviderFailed(t *testing.T) {
	app := coreruntime.New()
	startErr := errors.New("api start failed")
	if err := app.Register(testProvider{
		name:  "api",
		start: func(context.Context) error { return startErr },
		stop:  func(context.Context) error { return nil },
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.Run(context.Background()); !errors.Is(err, startErr) {
		t.Fatalf("Run() error = %v, want startErr", err)
	}
	health := app.ProviderStatus("api")
	if health.Status != coreruntime.StatusFailed {
		t.Fatalf("ProviderStatus(api) = %s, want failed", health.Status)
	}
	if !errors.Is(health.Error, startErr) {
		t.Fatalf("ProviderStatus(api).Error = %v, want startErr", health.Error)
	}
}

func TestPhase9ShutdownTimeoutMarksProviderFailed(t *testing.T) {
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
	health := app.ProviderStatus("slow")
	if health.Status != coreruntime.StatusFailed {
		t.Fatalf("ProviderStatus(slow) = %s, want failed", health.Status)
	}
	if !errors.Is(health.Error, context.DeadlineExceeded) {
		t.Fatalf("ProviderStatus(slow).Error = %v, want deadline exceeded", health.Error)
	}
}

func TestPhase9StatusLookupAndList(t *testing.T) {
	app := coreruntime.New()
	if err := app.Use(
		testCapability{name: "logger", register: func(*coreruntime.App) error { return nil }},
		testCapability{name: "database", register: func(*coreruntime.App) error { return nil }},
	); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Register(testProvider{name: "api"}, testProvider{name: "worker"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := app.ProviderStatus("api").Status; got != coreruntime.StatusRunning {
		t.Fatalf("ProviderStatus(api) = %s, want running", got)
	}
	if got := app.CapabilityStatus("database").Status; got != coreruntime.StatusRunning {
		t.Fatalf("CapabilityStatus(database) = %s, want running", got)
	}
	if got := healthNames(app.ProviderStatuses()); !reflect.DeepEqual(got, []string{"api", "worker"}) {
		t.Fatalf("ProviderStatuses names = %v, want [api worker]", got)
	}
	if got := healthStatuses(app.ProviderStatuses()); !reflect.DeepEqual(got, []coreruntime.Status{coreruntime.StatusRunning, coreruntime.StatusRunning}) {
		t.Fatalf("ProviderStatuses status = %v, want running/running", got)
	}
	if got := healthNames(app.CapabilityStatuses()); !reflect.DeepEqual(got, []string{"logger", "database"}) {
		t.Fatalf("CapabilityStatuses names = %v, want [logger database]", got)
	}
	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestPhase9DependencyFailureMarksRuntimeFailed(t *testing.T) {
	app := coreruntime.New()
	if err := app.Register(testProvider{
		name:         "api",
		dependencies: []coreruntime.Dependency{coreruntime.Require("database")},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.Run(context.Background()); !errors.Is(err, coreruntime.ErrDependencyMissing) {
		t.Fatalf("Run() error = %v, want ErrDependencyMissing", err)
	}
	health := app.ProviderStatus("api")
	if health.Status != coreruntime.StatusFailed {
		t.Fatalf("ProviderStatus(api) = %s, want failed", health.Status)
	}
	if !errors.Is(health.Error, coreruntime.ErrDependencyMissing) {
		t.Fatalf("ProviderStatus(api).Error = %v, want ErrDependencyMissing", health.Error)
	}
}

func healthNames(items []coreruntime.Health) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Name)
	}
	return out
}

func healthStatuses(items []coreruntime.Health) []coreruntime.Status {
	out := make([]coreruntime.Status, 0, len(items))
	for _, item := range items {
		out = append(out, item.Status)
	}
	return out
}
