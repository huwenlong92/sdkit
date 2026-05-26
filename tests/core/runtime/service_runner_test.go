package tests

import (
	"testing"

	coreruntime "github.com/huwenlong92/sdkit/core/runtime"
)

func TestServiceRunnerSelectsExplicitServices(t *testing.T) {
	enabled := true
	runner := coreruntime.NewServiceRunner(coreruntime.ServiceRunnerOptions[map[string]string]{
		LoadSpecs: func(string) (map[string]coreruntime.ServiceSpec, error) {
			return map[string]coreruntime.ServiceSpec{
				"api": {Type: "api", Enabled: &enabled},
			}, nil
		},
		Providers: []coreruntime.ServiceProvider[map[string]string]{
			coreruntime.ServiceProviderFunc[map[string]string](func(app *coreruntime.ServiceApp[map[string]string]) error {
				app.Service("api").Kind(coreruntime.ServiceKindHTTP).FactoryContext(func(coreruntime.ServiceContext[map[string]string]) (coreruntime.Service, error) {
					return testRuntimeService{info: coreruntime.ServiceInfo{Name: "api", Enabled: true}}, nil
				})
				return nil
			}),
		},
	})

	selection, err := runner.SelectServices(coreruntime.ServiceRunOptions{
		ConfigFile: "config.yaml",
		Services:   []string{"api"},
	})
	if err != nil {
		t.Fatalf("SelectServices() error = %v", err)
	}
	if len(selection.Services) != 1 {
		t.Fatalf("len(selection.Services) = %d, want 1", len(selection.Services))
	}
	if got := selection.Services[0]; got.Name != "api" || got.Type != "api" || got.Kind != coreruntime.ServiceKindHTTP {
		t.Fatalf("selection = %+v, want api/http", got)
	}
}

func TestServiceRunnerNewAppRegistersCapabilitiesAndProviders(t *testing.T) {
	runner := coreruntime.NewServiceRunner(coreruntime.ServiceRunnerOptions[map[string]string]{
		LoadSpecs: func(string) (map[string]coreruntime.ServiceSpec, error) {
			return map[string]coreruntime.ServiceSpec{
				"api": {Type: "api"},
			}, nil
		},
		Providers: []coreruntime.ServiceProvider[map[string]string]{
			coreruntime.ServiceProviderFunc[map[string]string](func(app *coreruntime.ServiceApp[map[string]string]) error {
				app.Service("api").Kind(coreruntime.ServiceKindHTTP).FactoryContext(func(coreruntime.ServiceContext[map[string]string]) (coreruntime.Service, error) {
					return testRuntimeService{info: coreruntime.ServiceInfo{Name: "api", Enabled: true}}, nil
				})
				return nil
			}),
		},
		Capabilities: func(coreruntime.ServiceSelection) []coreruntime.CapabilityContract {
			return []coreruntime.CapabilityContract{
				coreruntime.NewCapability("logger", func(*coreruntime.App) error { return nil }),
			}
		},
	})

	app, err := runner.NewApp(coreruntime.ServiceRunOptions{ConfigFile: "config.yaml"})
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
	runner := coreruntime.NewServiceRunner(coreruntime.ServiceRunnerOptions[map[string]string]{
		LoadSpecs: func(string) (map[string]coreruntime.ServiceSpec, error) {
			return map[string]coreruntime.ServiceSpec{
				"api": {Type: "api"},
			}, nil
		},
		Providers: []coreruntime.ServiceProvider[map[string]string]{
			coreruntime.ServiceProviderFunc[map[string]string](func(app *coreruntime.ServiceApp[map[string]string]) error {
				app.Service("api").
					Kind(coreruntime.ServiceKindHTTP).
					Group(coreruntime.GroupAPI).
					RequireCapabilities("database").
					FactoryContext(func(coreruntime.ServiceContext[map[string]string]) (coreruntime.Service, error) {
						return testRuntimeService{info: coreruntime.ServiceInfo{Name: "api", Enabled: true}}, nil
					})
				return nil
			}),
		},
	})

	selection, err := runner.SelectServices(coreruntime.ServiceRunOptions{ConfigFile: "config.yaml"})
	if err != nil {
		t.Fatalf("SelectServices() error = %v", err)
	}
	if len(selection.Services) != 1 {
		t.Fatalf("len(selection.Services) = %d, want 1", len(selection.Services))
	}
	item := selection.Services[0]
	if item.Group != coreruntime.GroupAPI {
		t.Fatalf("selection group = %s, want api", item.Group)
	}
	if len(item.Dependencies) != 1 || item.Dependencies[0].Name != "database" || !item.Dependencies[0].Required {
		t.Fatalf("selection dependencies = %+v, want required database", item.Dependencies)
	}

	app, err := runner.NewApp(coreruntime.ServiceRunOptions{ConfigFile: "config.yaml"})
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}
	provider, ok := app.Provider("api")
	if !ok {
		t.Fatal("api provider was not registered")
	}
	if provider.Metadata().Group != coreruntime.GroupAPI {
		t.Fatalf("provider group = %s, want api", provider.Metadata().Group)
	}
	deps := provider.Dependencies()
	if len(deps) != 1 || deps[0].Name != "database" || !deps[0].Required {
		t.Fatalf("provider dependencies = %+v, want required database", deps)
	}
}
