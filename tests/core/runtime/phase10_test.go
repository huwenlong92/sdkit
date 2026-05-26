package tests

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestPhase10AdaptersRegisterRuntimeContracts(t *testing.T) {
	app := runtime.New()
	capability := runtime.NewCapability("database", func(*runtime.App) error { return nil })
	provider := testProvider{name: "api"}
	command := testCommand{name: "serve"}

	if err := app.UseCapabilityAdapters(runtime.NewCapabilityAdapter(runtime.AdapterMetadata{
		Name: "bootstrap.database",
	}, capability)); err != nil {
		t.Fatalf("UseCapabilityAdapters() error = %v", err)
	}
	if err := app.RegisterProviderAdapters(runtime.NewProviderAdapter(runtime.AdapterMetadata{
		Name: "provider.api",
	}, provider)); err != nil {
		t.Fatalf("RegisterProviderAdapters() error = %v", err)
	}
	if err := app.RegisterCommandAdapters(runtime.NewCommandAdapter(runtime.AdapterMetadata{
		Name: "cobra.serve",
	}, command)); err != nil {
		t.Fatalf("RegisterCommandAdapters() error = %v", err)
	}

	if names := adapterNames(app.Adapters()); !reflect.DeepEqual(names, []string{"bootstrap.database", "provider.api", "cobra.serve"}) {
		t.Fatalf("Adapters() = %v, want bootstrap/provider/cobra adapters", names)
	}
	if names := adapterNames(app.AdaptersByType(runtime.AdapterTypeCommand)); !reflect.DeepEqual(names, []string{"cobra.serve"}) {
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
	app := runtime.New()
	if err := app.RegisterAdapters(testAdapter{}); !errors.Is(err, runtime.ErrAdapterNameRequired) {
		t.Fatalf("RegisterAdapters(empty name) error = %v, want ErrAdapterNameRequired", err)
	}
	if err := app.RegisterAdapters(testAdapter{metadata: runtime.AdapterMetadata{Name: "external"}}); !errors.Is(err, runtime.ErrAdapterTypeRequired) {
		t.Fatalf("RegisterAdapters(empty type) error = %v, want ErrAdapterTypeRequired", err)
	}
	if err := app.RegisterAdapters(testAdapter{metadata: runtime.AdapterMetadata{Name: "external", Type: "plugin"}}); !errors.Is(err, runtime.ErrAdapterTypeUnsupported) {
		t.Fatalf("RegisterAdapters(unsupported type) error = %v, want ErrAdapterTypeUnsupported", err)
	}
	if err := app.RegisterAdapters(
		testAdapter{metadata: runtime.AdapterMetadata{Name: "cobra.serve", Type: runtime.AdapterTypeCommand}},
		testAdapter{metadata: runtime.AdapterMetadata{Name: "cobra.serve", Type: runtime.AdapterTypeCommand}},
	); !errors.Is(err, runtime.ErrAdapterNameDuplicate) {
		t.Fatalf("RegisterAdapters(duplicate) error = %v, want ErrAdapterNameDuplicate", err)
	}
}

func TestPhase10PluginMetadataIntrospection(t *testing.T) {
	app := runtime.New()
	plugin := runtime.NewPlugin(runtime.PluginMetadata{
		Name:        "queue.redis",
		Description: "Redis queue integration",
		Group:       runtime.GroupSystem,
	})

	if err := app.RegisterPlugins(plugin); err != nil {
		t.Fatalf("RegisterPlugins() error = %v", err)
	}
	if names := pluginNames(app.Plugins()); !reflect.DeepEqual(names, []string{"queue.redis"}) {
		t.Fatalf("Plugins() = %v, want [queue.redis]", names)
	}
	if names := pluginNames(app.PluginsByGroup(runtime.GroupSystem)); !reflect.DeepEqual(names, []string{"queue.redis"}) {
		t.Fatalf("PluginsByGroup(system) = %v, want [queue.redis]", names)
	}
	got, ok := app.Plugin("queue.redis")
	if !ok {
		t.Fatal("Plugin(queue.redis) not found")
	}
	if metadata := got.Metadata(); metadata.Description == "" || metadata.Group != runtime.GroupSystem {
		t.Fatalf("plugin metadata = %+v, want system metadata", metadata)
	}
}

func TestPhase10PluginValidation(t *testing.T) {
	app := runtime.New()
	if err := app.RegisterPlugins(runtime.NewPlugin(runtime.PluginMetadata{})); !errors.Is(err, runtime.ErrPluginNameRequired) {
		t.Fatalf("RegisterPlugins(empty name) error = %v, want ErrPluginNameRequired", err)
	}
	if err := app.RegisterPlugins(runtime.NewPlugin(runtime.PluginMetadata{Name: "plugin"})); !errors.Is(err, runtime.ErrPluginNameReserved) {
		t.Fatalf("RegisterPlugins(reserved name) error = %v, want ErrPluginNameReserved", err)
	}
	if err := app.RegisterPlugins(
		runtime.NewPlugin(runtime.PluginMetadata{Name: "queue.redis"}),
		runtime.NewPlugin(runtime.PluginMetadata{Name: "queue.redis"}),
	); !errors.Is(err, runtime.ErrPluginNameDuplicate) {
		t.Fatalf("RegisterPlugins(duplicate) error = %v, want ErrPluginNameDuplicate", err)
	}
}

func TestPhase10RuntimeOwnsStatusForAdapterContracts(t *testing.T) {
	app := runtime.New()
	provider := externalStatusProvider{name: "api", status: runtime.StatusFailed}
	adapter := runtime.NewProviderAdapter(runtime.AdapterMetadata{Name: "provider.api"}, provider)
	if err := app.RegisterProviderAdapters(adapter); err != nil {
		t.Fatalf("RegisterProviderAdapters() error = %v", err)
	}

	if got := app.ProviderStatus("api").Status; got != runtime.StatusStopped {
		t.Fatalf("ProviderStatus(api) before Run = %s, want runtime-owned stopped", got)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := app.ProviderStatus("api").Status; got != runtime.StatusRunning {
		t.Fatalf("ProviderStatus(api) after Run = %s, want runtime-owned running", got)
	}
}

type testAdapter struct {
	metadata runtime.AdapterMetadata
}

func (a testAdapter) AdapterMetadata() runtime.AdapterMetadata {
	return a.metadata
}

type externalStatusProvider struct {
	name   string
	status runtime.Status
}

func (p externalStatusProvider) Name() string {
	return p.name
}

func (p externalStatusProvider) Metadata() runtime.ProviderMetadata {
	return runtime.ProviderMetadata{Name: p.name}
}

func (p externalStatusProvider) Dependencies() []runtime.Dependency {
	return nil
}

func (p externalStatusProvider) Register(*runtime.App) error {
	return nil
}

func (p externalStatusProvider) Start(context.Context) error {
	return nil
}

func (p externalStatusProvider) Stop(context.Context) error {
	return nil
}

func (p externalStatusProvider) Status() runtime.Status {
	return p.status
}

func adapterNames(adapters []runtime.Adapter) []string {
	names := make([]string, 0, len(adapters))
	for _, adapter := range adapters {
		names = append(names, adapter.AdapterMetadata().Name)
	}
	return names
}

func pluginNames(plugins []runtime.Plugin) []string {
	names := make([]string, 0, len(plugins))
	for _, plugin := range plugins {
		names = append(names, plugin.Metadata().Name)
	}
	return names
}
