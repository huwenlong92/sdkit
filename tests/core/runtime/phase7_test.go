package tests

import (
	"errors"
	"reflect"
	"testing"

	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestPhase7MetadataReturnsStableValues(t *testing.T) {
	capability := runtime.NewCapabilityWithMetadata(runtime.CapabilityMetadata{
		Name:        "database",
		Description: "Database connection capability",
		Group:       runtime.GroupSystem,
		Internal:    true,
	}, func(*runtime.App) error { return nil }, nil)

	if got := capability.Metadata(); got.Name != "database" || got.Description == "" || got.Group != runtime.GroupSystem || !got.Internal {
		t.Fatalf("capability metadata = %+v, want database/system/internal", got)
	}

	provider := testProvider{
		name: "api",
		metadata: runtime.ProviderMetadata{
			Name:        "api",
			Description: "API server",
			Group:       runtime.GroupAPI,
		},
	}
	if got := provider.Metadata(); got.Name != "api" || got.Description == "" || got.Group != runtime.GroupAPI || got.Internal {
		t.Fatalf("provider metadata = %+v, want api group", got)
	}

	command := testCommand{
		name: "serve",
		metadata: runtime.CommandMetadata{
			Name:        "serve",
			Description: "Serve all providers",
			Group:       runtime.GroupSystem,
		},
	}
	if got := command.Metadata(); got.Name != "serve" || got.Description == "" || got.Group != runtime.GroupSystem || got.Internal {
		t.Fatalf("command metadata = %+v, want serve system", got)
	}
}

func TestPhase7RegistryLookup(t *testing.T) {
	app := runtime.New()
	capability := runtime.NewCapability("database", func(*runtime.App) error { return nil })
	provider := testProvider{name: "api"}
	command := testCommand{name: "serve"}

	if err := app.Use(capability); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Register(provider); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := app.RegisterCommand(command); err != nil {
		t.Fatalf("RegisterCommand() error = %v", err)
	}

	if got, ok := app.Capability("database"); !ok || got.Name() != "database" {
		t.Fatalf("Capability(database) = %v, %v; want database, true", got, ok)
	}
	if got, ok := app.Provider("api"); !ok || got.Name() != "api" {
		t.Fatalf("Provider(api) = %v, %v; want api, true", got, ok)
	}
	if got, ok := app.Command("serve"); !ok || got.Name() != "serve" {
		t.Fatalf("Command(serve) = %v, %v; want serve, true", got, ok)
	}
}

func TestPhase7RegistryList(t *testing.T) {
	app := runtime.New()
	if err := app.Use(
		runtime.NewCapability("logger", func(*runtime.App) error { return nil }),
		runtime.NewCapability("database", func(*runtime.App) error { return nil }),
	); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Register(testProvider{name: "api"}, testProvider{name: "worker"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := app.RegisterCommand(testCommand{name: "run"}, testCommand{name: "serve"}); err != nil {
		t.Fatalf("RegisterCommand() error = %v", err)
	}

	if names := capabilityNames(app.Capabilities()); !reflect.DeepEqual(names, []string{"logger", "database"}) {
		t.Fatalf("Capabilities() = %v, want [logger database]", names)
	}
	if names := providerNames(app.Providers()); !reflect.DeepEqual(names, []string{"api", "worker"}) {
		t.Fatalf("Providers() = %v, want [api worker]", names)
	}
	if names := commandNames(app.Commands()); !reflect.DeepEqual(names, []string{"run", "serve"}) {
		t.Fatalf("Commands() = %v, want [run serve]", names)
	}
}

func TestPhase7RegistryGroup(t *testing.T) {
	app := runtime.New()
	if err := app.Use(
		testCapability{name: "logger", metadata: runtime.CapabilityMetadata{Name: "logger", Group: runtime.GroupSystem}},
		testCapability{name: "database", metadata: runtime.CapabilityMetadata{Name: "database", Group: runtime.GroupSystem}},
	); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Register(
		testProvider{name: "api", metadata: runtime.ProviderMetadata{Name: "api", Group: runtime.GroupAPI}},
		testProvider{name: "worker", metadata: runtime.ProviderMetadata{Name: "worker", Group: runtime.GroupWorker}},
	); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := app.RegisterCommand(
		testCommand{name: "run", metadata: runtime.CommandMetadata{Name: "run", Group: runtime.GroupSystem}},
		testCommand{name: "serve", metadata: runtime.CommandMetadata{Name: "serve", Group: runtime.GroupSystem}},
	); err != nil {
		t.Fatalf("RegisterCommand() error = %v", err)
	}

	if names := capabilityNames(app.CapabilitiesByGroup(runtime.GroupSystem)); !reflect.DeepEqual(names, []string{"logger", "database"}) {
		t.Fatalf("CapabilitiesByGroup(system) = %v, want [logger database]", names)
	}
	if names := providerNames(app.ProvidersByGroup(runtime.GroupAPI)); !reflect.DeepEqual(names, []string{"api"}) {
		t.Fatalf("ProvidersByGroup(api) = %v, want [api]", names)
	}
	if names := commandNames(app.CommandsByGroup(runtime.GroupSystem)); !reflect.DeepEqual(names, []string{"run", "serve"}) {
		t.Fatalf("CommandsByGroup(system) = %v, want [run serve]", names)
	}
}

func TestPhase7InternalObjectMetadata(t *testing.T) {
	app := runtime.New()
	if err := app.Register(testProvider{
		name: "runtime.internal",
		metadata: runtime.ProviderMetadata{
			Name:     "runtime.internal",
			Group:    runtime.GroupInternal,
			Internal: true,
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	provider, ok := app.Provider("runtime.internal")
	if !ok {
		t.Fatal("Provider(runtime.internal) not found")
	}
	metadata := provider.Metadata()
	if !metadata.Internal || metadata.Group != runtime.GroupInternal {
		t.Fatalf("provider metadata = %+v, want internal group", metadata)
	}
}

func TestPhase7RegistryRejectsInvalidNames(t *testing.T) {
	app := runtime.New()

	if err := app.Use(runtime.NewCapability("", func(*runtime.App) error { return nil })); !errors.Is(err, runtime.ErrCapabilityNameRequired) {
		t.Fatalf("Use(empty name) error = %v, want ErrCapabilityNameRequired", err)
	}
	if err := app.Use(runtime.NewCapability("capability", func(*runtime.App) error { return nil })); !errors.Is(err, runtime.ErrCapabilityNameReserved) {
		t.Fatalf("Use(reserved name) error = %v, want ErrCapabilityNameReserved", err)
	}
	if err := app.Use(
		runtime.NewCapability("database", func(*runtime.App) error { return nil }),
		runtime.NewCapability("database", func(*runtime.App) error { return nil }),
	); !errors.Is(err, runtime.ErrCapabilityNameDuplicate) {
		t.Fatalf("Use(duplicate capability) error = %v, want ErrCapabilityNameDuplicate", err)
	}
	if got := len(app.Capabilities()); got != 0 {
		t.Fatalf("capability count = %d, want 0", got)
	}
}
