package tests

import (
	"errors"
	"reflect"
	"testing"

	coreruntime "github.com/huwenlong92/sdkit/core/runtime"
)

func TestPhase7MetadataReturnsStableValues(t *testing.T) {
	capability := coreruntime.NewCapabilityWithMetadata(coreruntime.CapabilityMetadata{
		Name:        "database",
		Description: "Database connection capability",
		Group:       coreruntime.GroupSystem,
		Internal:    true,
	}, func(*coreruntime.App) error { return nil }, nil)

	if got := capability.Metadata(); got.Name != "database" || got.Description == "" || got.Group != coreruntime.GroupSystem || !got.Internal {
		t.Fatalf("capability metadata = %+v, want database/system/internal", got)
	}

	provider := testProvider{
		name: "api",
		metadata: coreruntime.ProviderMetadata{
			Name:        "api",
			Description: "API server",
			Group:       coreruntime.GroupAPI,
		},
	}
	if got := provider.Metadata(); got.Name != "api" || got.Description == "" || got.Group != coreruntime.GroupAPI || got.Internal {
		t.Fatalf("provider metadata = %+v, want api group", got)
	}

	command := testCommand{
		name: "serve",
		metadata: coreruntime.CommandMetadata{
			Name:        "serve",
			Description: "Serve all providers",
			Group:       coreruntime.GroupSystem,
		},
	}
	if got := command.Metadata(); got.Name != "serve" || got.Description == "" || got.Group != coreruntime.GroupSystem || got.Internal {
		t.Fatalf("command metadata = %+v, want serve system", got)
	}
}

func TestPhase7RegistryLookup(t *testing.T) {
	app := coreruntime.New()
	capability := coreruntime.NewCapability("database", func(*coreruntime.App) error { return nil })
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
	app := coreruntime.New()
	if err := app.Use(
		coreruntime.NewCapability("logger", func(*coreruntime.App) error { return nil }),
		coreruntime.NewCapability("database", func(*coreruntime.App) error { return nil }),
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
	app := coreruntime.New()
	if err := app.Use(
		testCapability{name: "logger", metadata: coreruntime.CapabilityMetadata{Name: "logger", Group: coreruntime.GroupSystem}},
		testCapability{name: "database", metadata: coreruntime.CapabilityMetadata{Name: "database", Group: coreruntime.GroupSystem}},
	); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Register(
		testProvider{name: "api", metadata: coreruntime.ProviderMetadata{Name: "api", Group: coreruntime.GroupAPI}},
		testProvider{name: "worker", metadata: coreruntime.ProviderMetadata{Name: "worker", Group: coreruntime.GroupWorker}},
	); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := app.RegisterCommand(
		testCommand{name: "run", metadata: coreruntime.CommandMetadata{Name: "run", Group: coreruntime.GroupSystem}},
		testCommand{name: "serve", metadata: coreruntime.CommandMetadata{Name: "serve", Group: coreruntime.GroupSystem}},
	); err != nil {
		t.Fatalf("RegisterCommand() error = %v", err)
	}

	if names := capabilityNames(app.CapabilitiesByGroup(coreruntime.GroupSystem)); !reflect.DeepEqual(names, []string{"logger", "database"}) {
		t.Fatalf("CapabilitiesByGroup(system) = %v, want [logger database]", names)
	}
	if names := providerNames(app.ProvidersByGroup(coreruntime.GroupAPI)); !reflect.DeepEqual(names, []string{"api"}) {
		t.Fatalf("ProvidersByGroup(api) = %v, want [api]", names)
	}
	if names := commandNames(app.CommandsByGroup(coreruntime.GroupSystem)); !reflect.DeepEqual(names, []string{"run", "serve"}) {
		t.Fatalf("CommandsByGroup(system) = %v, want [run serve]", names)
	}
}

func TestPhase7InternalObjectMetadata(t *testing.T) {
	app := coreruntime.New()
	if err := app.Register(testProvider{
		name: "runtime.internal",
		metadata: coreruntime.ProviderMetadata{
			Name:     "runtime.internal",
			Group:    coreruntime.GroupInternal,
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
	if !metadata.Internal || metadata.Group != coreruntime.GroupInternal {
		t.Fatalf("provider metadata = %+v, want internal group", metadata)
	}
}

func TestPhase7RegistryRejectsInvalidNames(t *testing.T) {
	app := coreruntime.New()

	if err := app.Use(coreruntime.NewCapability("", func(*coreruntime.App) error { return nil })); !errors.Is(err, coreruntime.ErrCapabilityNameRequired) {
		t.Fatalf("Use(empty name) error = %v, want ErrCapabilityNameRequired", err)
	}
	if err := app.Use(coreruntime.NewCapability("capability", func(*coreruntime.App) error { return nil })); !errors.Is(err, coreruntime.ErrCapabilityNameReserved) {
		t.Fatalf("Use(reserved name) error = %v, want ErrCapabilityNameReserved", err)
	}
	if err := app.Use(
		coreruntime.NewCapability("database", func(*coreruntime.App) error { return nil }),
		coreruntime.NewCapability("database", func(*coreruntime.App) error { return nil }),
	); !errors.Is(err, coreruntime.ErrCapabilityNameDuplicate) {
		t.Fatalf("Use(duplicate capability) error = %v, want ErrCapabilityNameDuplicate", err)
	}
	if got := len(app.Capabilities()); got != 0 {
		t.Fatalf("capability count = %d, want 0", got)
	}
}
