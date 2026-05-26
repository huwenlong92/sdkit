package tests

import (
	"testing"

	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestServiceRunnerSelectsExplicitServices(t *testing.T) {
	enabled := true
	runner := runtime.NewServiceRunner(runtime.ServiceRunnerOptions[map[string]string]{
		LoadSpecs: func(string) (map[string]runtime.ServiceSpec, error) {
			return map[string]runtime.ServiceSpec{
				"api": {Type: "api", Enabled: &enabled},
			}, nil
		},
		Providers: []runtime.ServiceProvider[map[string]string]{
			runtime.ServiceProviderFunc[map[string]string](func(app *runtime.ServiceApp[map[string]string]) error {
				app.Service("api").Kind(runtime.ServiceKindHTTP).FactoryContext(func(runtime.ServiceContext[map[string]string]) (runtime.Service, error) {
					return testRuntimeService{info: runtime.ServiceInfo{Name: "api", Enabled: true}}, nil
				})
				return nil
			}),
		},
	})

	selection, err := runner.SelectServices(runtime.ServiceRunOptions{
		ConfigFile: "config.yaml",
		Services:   []string{"api"},
	})
	if err != nil {
		t.Fatalf("SelectServices() error = %v", err)
	}
	if len(selection.Services) != 1 {
		t.Fatalf("len(selection.Services) = %d, want 1", len(selection.Services))
	}
	if got := selection.Services[0]; got.Name != "api" || got.Type != "api" || got.Kind != runtime.ServiceKindHTTP {
		t.Fatalf("selection = %+v, want api/http", got)
	}
}

func TestServiceRunnerNewAppRegistersCapabilitiesAndProviders(t *testing.T) {
	runner := runtime.NewServiceRunner(runtime.ServiceRunnerOptions[map[string]string]{
		LoadSpecs: func(string) (map[string]runtime.ServiceSpec, error) {
			return map[string]runtime.ServiceSpec{
				"api": {Type: "api"},
			}, nil
		},
		Providers: []runtime.ServiceProvider[map[string]string]{
			runtime.ServiceProviderFunc[map[string]string](func(app *runtime.ServiceApp[map[string]string]) error {
				app.Service("api").Kind(runtime.ServiceKindHTTP).FactoryContext(func(runtime.ServiceContext[map[string]string]) (runtime.Service, error) {
					return testRuntimeService{info: runtime.ServiceInfo{Name: "api", Enabled: true}}, nil
				})
				return nil
			}),
		},
		Capabilities: func(runtime.ServiceSelection) []runtime.CapabilityContract {
			return []runtime.CapabilityContract{
				runtime.NewCapability("logger", func(*runtime.App) error { return nil }),
			}
		},
	})

	app, err := runner.NewApp(runtime.ServiceRunOptions{ConfigFile: "config.yaml"})
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}
	if _, ok := app.Capability("logger"); !ok {
		t.Fatal("logger capability was not registered")
	}
	if _, ok := app.Provider("api"); !ok {
		t.Fatal("api provider was not registered")
	}
}

func TestServiceRunnerUsesServiceDefinitionMetadata(t *testing.T) {
	runner := runtime.NewServiceRunner(runtime.ServiceRunnerOptions[map[string]string]{
		LoadSpecs: func(string) (map[string]runtime.ServiceSpec, error) {
			return map[string]runtime.ServiceSpec{
				"api": {Type: "api"},
			}, nil
		},
		Providers: []runtime.ServiceProvider[map[string]string]{
			runtime.ServiceProviderFunc[map[string]string](func(app *runtime.ServiceApp[map[string]string]) error {
				app.Service("api").
					Kind(runtime.ServiceKindHTTP).
					Group(runtime.GroupAPI).
					RequireCapabilities("database").
					FactoryContext(func(runtime.ServiceContext[map[string]string]) (runtime.Service, error) {
						return testRuntimeService{info: runtime.ServiceInfo{Name: "api", Enabled: true}}, nil
					})
				return nil
			}),
		},
	})

	selection, err := runner.SelectServices(runtime.ServiceRunOptions{ConfigFile: "config.yaml"})
	if err != nil {
		t.Fatalf("SelectServices() error = %v", err)
	}
	if len(selection.Services) != 1 {
		t.Fatalf("len(selection.Services) = %d, want 1", len(selection.Services))
	}
	item := selection.Services[0]
	if item.Group != runtime.GroupAPI {
		t.Fatalf("selection group = %s, want api", item.Group)
	}
	if len(item.Dependencies) != 1 || item.Dependencies[0].Name != "database" || !item.Dependencies[0].Required {
		t.Fatalf("selection dependencies = %+v, want required database", item.Dependencies)
	}

	app, err := runner.NewApp(runtime.ServiceRunOptions{ConfigFile: "config.yaml"})
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}
	provider, ok := app.Provider("api")
	if !ok {
		t.Fatal("api provider was not registered")
	}
	if provider.Metadata().Group != runtime.GroupAPI {
		t.Fatalf("provider group = %s, want api", provider.Metadata().Group)
	}
	deps := provider.Dependencies()
	if len(deps) != 1 || deps[0].Name != "database" || !deps[0].Required {
		t.Fatalf("provider dependencies = %+v, want required database", deps)
	}
}
