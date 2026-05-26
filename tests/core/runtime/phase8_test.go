package tests

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestPhase8DependencyValidation(t *testing.T) {
	t.Run("missing dependency", func(t *testing.T) {
		app := runtime.New()
		if err := app.Use(testCapability{
			name:         "queue",
			dependencies: []runtime.Dependency{runtime.Require("redis")},
		}); err != nil {
			t.Fatalf("Use() error = %v", err)
		}

		if err := app.ValidateDependencies(); !errors.Is(err, runtime.ErrDependencyMissing) {
			t.Fatalf("ValidateDependencies() error = %v, want ErrDependencyMissing", err)
		}
	})

	t.Run("duplicate dependency", func(t *testing.T) {
		app := runtime.New()
		if err := app.Use(
			testCapability{name: "redis"},
			testCapability{
				name: "queue",
				dependencies: []runtime.Dependency{
					runtime.Require("redis"),
					runtime.Optional("redis"),
				},
			},
		); err != nil {
			t.Fatalf("Use() error = %v", err)
		}

		if err := app.ValidateDependencies(); !errors.Is(err, runtime.ErrDependencyDuplicate) {
			t.Fatalf("ValidateDependencies() error = %v, want ErrDependencyDuplicate", err)
		}
	})

	t.Run("cycle dependency", func(t *testing.T) {
		app := runtime.New()
		if err := app.Use(
			testCapability{name: "redis", dependencies: []runtime.Dependency{runtime.Require("queue")}},
			testCapability{name: "queue", dependencies: []runtime.Dependency{runtime.Require("redis")}},
		); err != nil {
			t.Fatalf("Use() error = %v", err)
		}

		if err := app.ValidateDependencies(); !errors.Is(err, runtime.ErrDependencyCycle) {
			t.Fatalf("ValidateDependencies() error = %v, want ErrDependencyCycle", err)
		}
	})
}

func TestPhase8BootOrderAndShutdownOrder(t *testing.T) {
	app := runtime.New()
	var calls []string
	if err := app.Use(
		testCapability{
			name:         "queue",
			dependencies: []runtime.Dependency{runtime.Require("redis")},
			register: func(*runtime.App) error {
				calls = append(calls, "queue.register")
				return nil
			},
			shutdown: func(context.Context) error {
				calls = append(calls, "queue.shutdown")
				return nil
			},
		},
		testCapability{
			name: "redis",
			register: func(*runtime.App) error {
				calls = append(calls, "redis.register")
				return nil
			},
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
			name:         "worker",
			dependencies: []runtime.Dependency{runtime.Require("queue")},
			register: func(*runtime.App) error {
				calls = append(calls, "worker.register")
				return nil
			},
			start: func(context.Context) error {
				calls = append(calls, "worker.start")
				return nil
			},
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
		"redis.register",
		"queue.register",
		"worker.register",
		"worker.start",
		"worker.stop",
		"queue.shutdown",
		"redis.shutdown",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestPhase8ProviderBootOrder(t *testing.T) {
	app := runtime.New()
	var calls []string
	if err := app.Use(testCapability{
		name:     "redis",
		register: func(*runtime.App) error { calls = append(calls, "redis.register"); return nil },
	}); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Register(
		testProvider{
			name:         "worker",
			dependencies: []runtime.Dependency{runtime.Require("queue")},
			start:        func(context.Context) error { calls = append(calls, "worker.start"); return nil },
			stop:         func(context.Context) error { calls = append(calls, "worker.stop"); return nil },
		},
		testProvider{
			name:         "queue",
			dependencies: []runtime.Dependency{runtime.Require("redis")},
			start:        func(context.Context) error { calls = append(calls, "queue.start"); return nil },
			stop:         func(context.Context) error { calls = append(calls, "queue.stop"); return nil },
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
		"redis.register",
		"queue.start",
		"worker.start",
		"worker.stop",
		"queue.stop",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestPhase8RunProviderStartsProviderDependencies(t *testing.T) {
	app := runtime.New()
	var calls []string
	if err := app.Register(
		testProvider{
			name:         "worker",
			dependencies: []runtime.Dependency{runtime.Require("queue")},
			start:        func(context.Context) error { calls = append(calls, "worker.start"); return nil },
			stop:         func(context.Context) error { calls = append(calls, "worker.stop"); return nil },
		},
		testProvider{
			name:  "queue",
			start: func(context.Context) error { calls = append(calls, "queue.start"); return nil },
			stop:  func(context.Context) error { calls = append(calls, "queue.stop"); return nil },
		},
		testProvider{
			name:  "api",
			start: func(context.Context) error { calls = append(calls, "api.start"); return nil },
			stop:  func(context.Context) error { calls = append(calls, "api.stop"); return nil },
		},
	); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := runtime.RunProvider(context.Background(), app, "worker"); err != nil {
		t.Fatalf("RunProvider(worker) error = %v", err)
	}

	want := []string{"queue.start", "worker.start", "worker.stop", "queue.stop"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestPhase8OptionalDependencyAndLookup(t *testing.T) {
	app := runtime.New()
	var calls []string
	if err := app.Use(testCapability{
		name:         "cache",
		dependencies: []runtime.Dependency{runtime.Optional("redis")},
		register: func(*runtime.App) error {
			calls = append(calls, "cache.register")
			return nil
		},
	}); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Register(testProvider{
		name:         "api",
		dependencies: []runtime.Dependency{runtime.Require("cache")},
		start:        func(context.Context) error { calls = append(calls, "api.start"); return nil },
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if got := app.Dependencies(); !reflect.DeepEqual(got, []runtime.DependencyMetadata{
		{Source: "cache", Target: "redis", Required: false},
		{Source: "api", Target: "cache", Required: true},
	}) {
		t.Fatalf("Dependencies() = %+v, want cache/api dependency metadata", got)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"cache.register", "api.start"}) {
		t.Fatalf("calls = %v, want cache register then api start", calls)
	}
}
