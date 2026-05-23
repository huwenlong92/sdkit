package tests

import (
	"context"
	"errors"
	"reflect"
	"testing"

	coreruntime "github.com/huwenlong92/sdkit/core/runtime"
)

func TestPhase10AdaptersRegisterRuntimeContracts(t *testing.T) {
	app := coreruntime.New()
	capability := coreruntime.NewCapability("database", func(*coreruntime.App) error { return nil })
	provider := testProvider{name: "api"}
	command := testCommand{name: "serve"}

	if err := app.UseCapabilityAdapters(coreruntime.NewCapabilityAdapter(coreruntime.AdapterMetadata{
		Name: "bootstrap.database",
	}, capability)); err != nil {
		t.Fatalf("UseCapabilityAdapters() error = %v", err)
	}
	if err := app.RegisterProviderAdapters(coreruntime.NewProviderAdapter(coreruntime.AdapterMetadata{
		Name: "provider.api",
	}, provider)); err != nil {
		t.Fatalf("RegisterProviderAdapters() error = %v", err)
	}
	if err := app.RegisterCommandAdapters(coreruntime.NewCommandAdapter(coreruntime.AdapterMetadata{
		Name: "cobra.serve",
	}, command)); err != nil {
		t.Fatalf("RegisterCommandAdapters() error = %v", err)
	}

	if names := adapterNames(app.Adapters()); !reflect.DeepEqual(names, []string{"bootstrap.database", "provider.api", "cobra.serve"}) {
		t.Fatalf("Adapters() = %v, want bootstrap/provider/cobra adapters", names)
	}
	if names := adapterNames(app.AdaptersByType(coreruntime.AdapterTypeCommand)); !reflect.DeepEqual(names, []string{"cobra.serve"}) {
		t.Fatalf("AdaptersByType(command) = %v, want [cobra.serve]", names)
	}
	if _, ok := app.Capability("database"); !ok {
		t.Fatal("capability contract was not registered through adapter")
	}
	if _, ok := app.Provider("api"); !ok {
		t.Fatal("provider contract was not registered through adapter")
	}
	if _, ok := app.Command("serve"); !ok {
		t.Fatal("command contract was not registered through adapter")
	}
}

func TestPhase10AdapterValidation(t *testing.T) {
	app := coreruntime.New()
	if err := app.RegisterAdapters(testAdapter{}); !errors.Is(err, coreruntime.ErrAdapterNameRequired) {
		t.Fatalf("RegisterAdapters(empty name) error = %v, want ErrAdapterNameRequired", err)
	}
	if err := app.RegisterAdapters(testAdapter{metadata: coreruntime.AdapterMetadata{Name: "external"}}); !errors.Is(err, coreruntime.ErrAdapterTypeRequired) {
		t.Fatalf("RegisterAdapters(empty type) error = %v, want ErrAdapterTypeRequired", err)
	}
	if err := app.RegisterAdapters(testAdapter{metadata: coreruntime.AdapterMetadata{Name: "external", Type: "plugin"}}); !errors.Is(err, coreruntime.ErrAdapterTypeUnsupported) {
		t.Fatalf("RegisterAdapters(unsupported type) error = %v, want ErrAdapterTypeUnsupported", err)
	}
	if err := app.RegisterAdapters(
		testAdapter{metadata: coreruntime.AdapterMetadata{Name: "cobra.serve", Type: coreruntime.AdapterTypeCommand}},
		testAdapter{metadata: coreruntime.AdapterMetadata{Name: "cobra.serve", Type: coreruntime.AdapterTypeCommand}},
	); !errors.Is(err, coreruntime.ErrAdapterNameDuplicate) {
		t.Fatalf("RegisterAdapters(duplicate) error = %v, want ErrAdapterNameDuplicate", err)
	}
}

func TestPhase10PluginMetadataIntrospection(t *testing.T) {
	app := coreruntime.New()
	plugin := coreruntime.NewPlugin(coreruntime.PluginMetadata{
		Name:        "queue.redis",
		Description: "Redis queue integration",
		Group:       coreruntime.GroupSystem,
	})

	if err := app.RegisterPlugins(plugin); err != nil {
		t.Fatalf("RegisterPlugins() error = %v", err)
	}
	if names := pluginNames(app.Plugins()); !reflect.DeepEqual(names, []string{"queue.redis"}) {
		t.Fatalf("Plugins() = %v, want [queue.redis]", names)
	}
	if names := pluginNames(app.PluginsByGroup(coreruntime.GroupSystem)); !reflect.DeepEqual(names, []string{"queue.redis"}) {
		t.Fatalf("PluginsByGroup(system) = %v, want [queue.redis]", names)
	}
	got, ok := app.Plugin("queue.redis")
	if !ok {
		t.Fatal("Plugin(queue.redis) not found")
	}
	if metadata := got.Metadata(); metadata.Description == "" || metadata.Group != coreruntime.GroupSystem {
		t.Fatalf("plugin metadata = %+v, want system metadata", metadata)
	}
}

func TestPhase10PluginValidation(t *testing.T) {
	app := coreruntime.New()
	if err := app.RegisterPlugins(coreruntime.NewPlugin(coreruntime.PluginMetadata{})); !errors.Is(err, coreruntime.ErrPluginNameRequired) {
		t.Fatalf("RegisterPlugins(empty name) error = %v, want ErrPluginNameRequired", err)
	}
	if err := app.RegisterPlugins(coreruntime.NewPlugin(coreruntime.PluginMetadata{Name: "plugin"})); !errors.Is(err, coreruntime.ErrPluginNameReserved) {
		t.Fatalf("RegisterPlugins(reserved name) error = %v, want ErrPluginNameReserved", err)
	}
	if err := app.RegisterPlugins(
		coreruntime.NewPlugin(coreruntime.PluginMetadata{Name: "queue.redis"}),
		coreruntime.NewPlugin(coreruntime.PluginMetadata{Name: "queue.redis"}),
	); !errors.Is(err, coreruntime.ErrPluginNameDuplicate) {
		t.Fatalf("RegisterPlugins(duplicate) error = %v, want ErrPluginNameDuplicate", err)
	}
}

func TestPhase10RuntimeOwnsStatusForAdapterContracts(t *testing.T) {
	app := coreruntime.New()
	provider := externalStatusProvider{name: "api", status: coreruntime.StatusFailed}
	adapter := coreruntime.NewProviderAdapter(coreruntime.AdapterMetadata{Name: "provider.api"}, provider)
	if err := app.RegisterProviderAdapters(adapter); err != nil {
		t.Fatalf("RegisterProviderAdapters() error = %v", err)
	}

	if got := app.ProviderStatus("api").Status; got != coreruntime.StatusStopped {
		t.Fatalf("ProviderStatus(api) before Run = %s, want runtime-owned stopped", got)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := app.ProviderStatus("api").Status; got != coreruntime.StatusRunning {
		t.Fatalf("ProviderStatus(api) after Run = %s, want runtime-owned running", got)
	}
}

type testAdapter struct {
	metadata coreruntime.AdapterMetadata
}

func (a testAdapter) AdapterMetadata() coreruntime.AdapterMetadata {
	return a.metadata
}

type externalStatusProvider struct {
	name   string
	status coreruntime.Status
}

func (p externalStatusProvider) Name() string {
	return p.name
}

func (p externalStatusProvider) Metadata() coreruntime.ProviderMetadata {
	return coreruntime.ProviderMetadata{Name: p.name}
}

func (p externalStatusProvider) Dependencies() []coreruntime.Dependency {
	return nil
}

func (p externalStatusProvider) Register(*coreruntime.App) error {
	return nil
}

func (p externalStatusProvider) Start(context.Context) error {
	return nil
}

func (p externalStatusProvider) Stop(context.Context) error {
	return nil
}

func (p externalStatusProvider) Status() coreruntime.Status {
	return p.status
}

func adapterNames(adapters []coreruntime.Adapter) []string {
	names := make([]string, 0, len(adapters))
	for _, adapter := range adapters {
		names = append(names, adapter.AdapterMetadata().Name)
	}
	return names
}

func pluginNames(plugins []coreruntime.Plugin) []string {
	names := make([]string, 0, len(plugins))
	for _, plugin := range plugins {
		names = append(names, plugin.Metadata().Name)
	}
	return names
}
