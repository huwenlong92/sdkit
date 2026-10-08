package sysprobe_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/runtime"
	"github.com/huwenlong92/sdkit/core/sysprobe"
	"github.com/huwenlong92/sdkit/core/sysprobe/facade"
)

func TestFacadeOwnsSeparateInstancesAndDefault(t *testing.T) {
	firstApp, secondApp := runtime.New(), runtime.New()
	loads := 0
	first := facade.Use(facade.WithName("first-probe"), facade.WithDefault(), facade.WithConfigLoader(func(*runtime.App) (facade.Config, error) {
		loads++
		return facade.Config{Host: facade.HostConfig{CPUInterval: time.Millisecond}}, nil
	}))
	second := facade.Use(facade.WithName("second-probe"), facade.WithDefault(), facade.WithConfig(facade.Config{Host: facade.HostConfig{CPUInterval: time.Millisecond}}))
	for _, cap := range []runtime.Capability{first, second} {
		t.Cleanup(func() {
			if err := cap.Shutdown(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	if err := first.Register(firstApp); err != nil {
		t.Fatal(err)
	}
	a := facade.From(firstApp, "first-probe")
	if a == nil || facade.FromDefault() != a {
		t.Fatal("first service was not bound as its own default")
	}
	ctx := runtime.NewServiceContext("admin", "http", (*int)(nil))
	ctx.Capabilities.Set("admin."+facade.Name, a)
	if facade.FromServiceContext(ctx) != a || facade.FromServiceContext[*int](nil) != nil {
		t.Fatal("service context did not resolve its local probe")
	}
	if err := first.Register(firstApp); err != nil {
		t.Fatal(err)
	}
	if loads != 1 || facade.From(firstApp, "first-probe") != a {
		t.Fatalf("loads=%d, want one initialization and same instance", loads)
	}
	if err := second.Register(secondApp); err != nil {
		t.Fatal(err)
	}
	b := facade.From(secondApp, "second-probe")
	if b == nil || a == b || facade.FromDefault() != a {
		t.Fatal("second service replaced the first service or default")
	}
	if err := first.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if facade.FromDefault() != nil {
		t.Fatal("owned default was not cleared")
	}
	if _, err := a.Snapshot(context.Background()); !errors.Is(err, sysprobe.ErrUnavailable) {
		t.Fatalf("first error=%v, want closed", err)
	}
	if _, err := b.Snapshot(context.Background()); err != nil {
		t.Fatalf("second service was closed by first: %v", err)
	}
	if err := first.Register(firstApp); err != nil {
		t.Fatal(err)
	}
	if facade.FromDefault() == nil || facade.FromDefault() == a || loads != 2 {
		t.Fatalf("loads=%d, want fresh initialization after shutdown", loads)
	}
}

func TestFacadeConfigurationFailures(t *testing.T) {
	cause := errors.New("configuration failed")
	tests := []struct {
		name       string
		capability func() runtime.Capability
		app        *runtime.App
		wantCause  error
	}{
		{name: "missing_loader", capability: func() runtime.Capability { return facade.Use() }, app: runtime.New()},
		{name: "loader_error", capability: func() runtime.Capability {
			return facade.Use(facade.WithConfigLoader(func(*runtime.App) (facade.Config, error) { return facade.Config{}, cause }))
		}, app: runtime.New(), wantCause: cause},
		{name: "nil_app", capability: func() runtime.Capability { return facade.Use(facade.WithConfig(facade.Config{})) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cap := test.capability()
			err := cap.Register(test.app)
			if err == nil || (test.wantCause != nil && !errors.Is(err, test.wantCause)) {
				t.Fatalf("error=%v, want non-nil with cause=%v", err, test.wantCause)
			}
			if facade.From(test.app) != nil {
				t.Fatal("failed initialization bound a service")
			}
		})
	}
}

func TestFacadeReadsRequiredConfiguration(t *testing.T) {
	tests := []struct {
		name, contents string
		wantError      bool
	}{
		{name: "valid", contents: "sysprobe:\n  host:\n    cpu_interval: 1ms\n  network_capacity:\n    send_limit_mbps: 7\n"},
		{name: "missing", contents: "other: true\n", wantError: true},
		{name: "malformed", contents: "sysprobe: [\n", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(test.contents), 0600); err != nil {
				t.Fatal(err)
			}
			cap := facade.UseConfigFile(path)
			t.Cleanup(func() {
				if err := cap.Shutdown(context.Background()); err != nil {
					t.Error(err)
				}
			})
			app := runtime.New()
			err := cap.Register(app)
			if test.wantError {
				if err == nil {
					t.Fatal("expected configuration error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			service := facade.From(app)
			if service == nil {
				t.Fatal("service missing from container")
			}
			snapshot, err := service.Snapshot(context.Background())
			if err != nil || snapshot.NetworkCapacity.SendLimitMbps != 7 {
				t.Fatalf("snapshot=%+v error=%v, want configured capacity", snapshot, err)
			}
		})
	}
}
