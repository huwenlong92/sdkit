package tests

import (
	"context"
	"reflect"
	"testing"

	coreruntime "github.com/huwenlong92/sdkit/core/runtime"
)

func TestCapabilityResetRegisterCapabilitiesAndScope(t *testing.T) {
	app := coreruntime.New()
	if err := app.RegisterCapabilities(
		testCapability{name: "database"},
		testCapability{
			name: "openai",
			metadata: coreruntime.CapabilityMetadata{
				Name:  "openai",
				Scope: coreruntime.ScopeServiceLocal,
			},
		},
	); err != nil {
		t.Fatalf("RegisterCapabilities() error = %v", err)
	}

	if names := capabilityNames(app.CapabilitiesByScope(coreruntime.ScopeGlobal)); !reflect.DeepEqual(names, []string{"database"}) {
		t.Fatalf("global capabilities = %v, want [database]", names)
	}
	if names := capabilityNames(app.CapabilitiesByScope(coreruntime.ScopeServiceLocal)); !reflect.DeepEqual(names, []string{"openai"}) {
		t.Fatalf("service local capabilities = %v, want [openai]", names)
	}
}

func TestCapabilityResetRequireCapabilities(t *testing.T) {
	app := coreruntime.New()
	if err := app.RegisterCapabilities(
		testCapability{name: "database"},
		testCapability{name: "queue"},
	); err != nil {
		t.Fatalf("RegisterCapabilities() error = %v", err)
	}
	if err := app.Register(testProvider{
		name:         "worker",
		dependencies: coreruntime.RequireCapabilities("database", "queue"),
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.ValidateDependencies(); err != nil {
		t.Fatalf("ValidateDependencies() error = %v", err)
	}
	want := []coreruntime.DependencyMetadata{
		{Source: "worker", Target: "database", Required: true},
		{Source: "worker", Target: "queue", Required: true},
	}
	if got := app.Dependencies(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Dependencies() = %+v, want %+v", got, want)
	}
}

func TestCapabilityResetServiceLocalLifecycle(t *testing.T) {
	app := coreruntime.New()
	var calls []string
	if err := app.RegisterCapabilities(
		testCapability{
			name:     "logger",
			register: func(*coreruntime.App) error { calls = append(calls, "logger.register"); return nil },
			shutdown: func(context.Context) error {
				calls = append(calls, "logger.shutdown")
				return nil
			},
		},
		testCapability{
			name: "openai",
			metadata: coreruntime.CapabilityMetadata{
				Name:  "openai",
				Scope: coreruntime.ScopeServiceLocal,
			},
			dependencies: coreruntime.RequireCapabilities("logger"),
			register:     func(*coreruntime.App) error { calls = append(calls, "openai.register"); return nil },
			shutdown: func(context.Context) error {
				calls = append(calls, "openai.shutdown")
				return nil
			},
		},
	); err != nil {
		t.Fatalf("RegisterCapabilities() error = %v", err)
	}
	if err := app.Register(testProvider{
		name:         "api",
		dependencies: coreruntime.RequireCapabilities("openai"),
		start:        func(context.Context) error { calls = append(calls, "api.start"); return nil },
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	want := []string{
		"logger.register",
		"openai.register",
		"api.start",
		"openai.shutdown",
		"logger.shutdown",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestCapabilityResetProviderRuntimeCapabilities(t *testing.T) {
	app := coreruntime.New()
	var calls []string
	openaiKey := coreruntime.Key("api.openai")
	openaiCapability := testCapability{
		name: "api.openai",
		metadata: coreruntime.CapabilityMetadata{
			Name:  "api.openai",
			Group: coreruntime.GroupAPI,
			Scope: coreruntime.ScopeServiceLocal,
		},
		dependencies: coreruntime.RequireCapabilities("logger"),
		register: func(app *coreruntime.App) error {
			calls = append(calls, "openai.register")
			return app.Container().Bind(openaiKey, "client")
		},
		shutdown: func(context.Context) error {
			calls = append(calls, "openai.shutdown")
			return nil
		},
	}

	if err := app.RegisterCapabilities(testCapability{
		name:     "logger",
		register: func(*coreruntime.App) error { calls = append(calls, "logger.register"); return nil },
		shutdown: func(context.Context) error {
			calls = append(calls, "logger.shutdown")
			return nil
		},
	}); err != nil {
		t.Fatalf("RegisterCapabilities() error = %v", err)
	}
	if err := app.Register(testProvider{
		name:                "api",
		dependencies:        coreruntime.RequireCapabilities("api.openai"),
		runtimeCapabilities: []coreruntime.CapabilityContract{openaiCapability},
		register: func(app *coreruntime.App) error {
			if got := app.Container().MustGet(openaiKey); got != "client" {
				t.Fatalf("openai client = %v, want client", got)
			}
			calls = append(calls, "api.register")
			return nil
		},
		start: func(context.Context) error {
			calls = append(calls, "api.start")
			return nil
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.ValidateDependencies(); err != nil {
		t.Fatalf("ValidateDependencies() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if names := capabilityNames(app.CapabilitiesByScope(coreruntime.ScopeServiceLocal)); !reflect.DeepEqual(names, []string{"api.openai"}) {
		t.Fatalf("service local capabilities = %v, want [api.openai]", names)
	}
	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	want := []string{
		"logger.register",
		"openai.register",
		"api.register",
		"api.start",
		"openai.shutdown",
		"logger.shutdown",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestCapabilityResetRunProviderCollectsOnlySelectedProviderCapabilities(t *testing.T) {
	app := coreruntime.New()
	var calls []string
	if err := app.Register(
		testProvider{
			name:         "api",
			dependencies: coreruntime.RequireCapabilities("api.openai"),
			runtimeCapabilities: []coreruntime.CapabilityContract{
				testCapability{
					name: "api.openai",
					metadata: coreruntime.CapabilityMetadata{
						Name:  "api.openai",
						Group: coreruntime.GroupAPI,
						Scope: coreruntime.ScopeServiceLocal,
					},
					register: func(*coreruntime.App) error {
						calls = append(calls, "openai.register")
						return nil
					},
				},
			},
			start: func(context.Context) error {
				calls = append(calls, "api.start")
				return nil
			},
		},
		testProvider{
			name:         "worker",
			dependencies: coreruntime.RequireCapabilities("worker.sms"),
			runtimeCapabilities: []coreruntime.CapabilityContract{
				testCapability{
					name: "worker.sms",
					metadata: coreruntime.CapabilityMetadata{
						Name:  "worker.sms",
						Group: coreruntime.GroupWorker,
						Scope: coreruntime.ScopeServiceLocal,
					},
					register: func(*coreruntime.App) error {
						calls = append(calls, "sms.register")
						return nil
					},
				},
			},
			start: func(context.Context) error {
				calls = append(calls, "worker.start")
				return nil
			},
		},
	); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	capabilityProvider, ok := app.Providers()[0].(coreruntime.RuntimeCapabilityProvider)
	if !ok || len(capabilityProvider.RuntimeCapabilities()) != 1 {
		t.Fatalf("api provider runtime capabilities = %v, want one", capabilityProvider)
	}

	if err := app.RunProvider("api", context.Background()); err != nil {
		t.Fatalf("RunProvider(api) error = %v", err)
	}
	if _, ok := app.Capability("api.openai"); !ok {
		t.Fatal("api.openai capability not registered")
	}
	if _, ok := app.Capability("worker.sms"); ok {
		t.Fatal("worker.sms capability should not be registered when running api only")
	}
	if want := []string{"openai.register", "api.start"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestCapabilityResetProviderRuntimeCapabilitiesAreIdempotentAcrossRuns(t *testing.T) {
	app := coreruntime.New()
	var registerCount int
	if err := app.Register(testProvider{
		name:         "api",
		dependencies: coreruntime.RequireCapabilities("api.openai"),
		runtimeCapabilities: []coreruntime.CapabilityContract{
			testCapability{
				name: "api.openai",
				register: func(*coreruntime.App) error {
					registerCount++
					return nil
				},
			},
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if registerCount != 2 {
		t.Fatalf("register count = %d, want 2", registerCount)
	}
}
