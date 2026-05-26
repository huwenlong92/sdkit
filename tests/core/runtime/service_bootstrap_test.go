package tests

import (
	"errors"
	"reflect"
	"testing"

	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestServiceBootstrapBuildsServicesFromLoader(t *testing.T) {
	base := map[string]string{"app": "demo"}
	bootstrap := runtime.NewServiceBootstrap[map[string]string](func(configFile string) (map[string]runtime.ServiceSpec, error) {
		if configFile != "config.yaml" {
			t.Fatalf("configFile = %q, want config.yaml", configFile)
		}
		return map[string]runtime.ServiceSpec{
			"public": {
				Type:      "api",
				ConfigKey: "api_public",
			},
		}, nil
	})
	bootstrap.RegisterServiceDefinition(runtime.ServiceDefinition[map[string]string]{
		Type: "api",
		Kind: runtime.ServiceKindHTTP,
		ContextFactory: func(ctx runtime.ServiceContext[map[string]string]) (runtime.Service, error) {
			if ctx.ConfigFile != "config.yaml" || ctx.Name != "public" || ctx.Type != "api" || ctx.ConfigKey != "api_public" {
				t.Fatalf("ServiceContext = %+v", ctx)
			}
			if ctx.Base["app"] != "demo" {
				t.Fatalf("ServiceContext.Base = %v, want demo", ctx.Base)
			}
			return testRuntimeService{info: runtime.ServiceInfo{Name: "public", Enabled: true}}, nil
		},
	})

	services, err := bootstrap.BuildServices("config.yaml", base)
	if err != nil {
		t.Fatalf("BuildServices() error = %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("len(services) = %d, want 1", len(services))
	}
	info := services[0].ServiceInfo()
	if info.Type != "api" || info.Kind != runtime.ServiceKindHTTP {
		t.Fatalf("ServiceInfo() = %+v, want type api kind http", info)
	}
}

func TestServiceBootstrapUsesResolverForSingleServiceConfigKey(t *testing.T) {
	bootstrap := runtime.NewServiceBootstrapWithResolver[map[string]string](
		func(string) (map[string]runtime.ServiceSpec, error) {
			return nil, errors.New("load all specs should not run")
		},
		func(configFile string, name string) (runtime.ServiceSpec, error) {
			if configFile != "config.yaml" || name != "admin" {
				t.Fatalf("resolver got configFile=%q name=%q", configFile, name)
			}
			return runtime.ServiceSpec{ConfigKey: "admin_service"}, nil
		},
	)

	configKey, err := bootstrap.ResolveServiceConfigKey("config.yaml", "admin")
	if err != nil {
		t.Fatalf("ResolveServiceConfigKey() error = %v", err)
	}
	if configKey != "admin_service" {
		t.Fatalf("configKey = %q, want admin_service", configKey)
	}
}

func TestServiceBootstrapRegistersProvider(t *testing.T) {
	bootstrap := runtime.NewServiceBootstrap[map[string]string](func(string) (map[string]runtime.ServiceSpec, error) {
		return nil, nil
	})
	err := bootstrap.RegisterProvider(runtime.ServiceProviderFunc[map[string]string](func(app *runtime.ServiceApp[map[string]string]) error {
		app.Service("api").Kind(runtime.ServiceKindHTTP).FactoryContext(func(runtime.ServiceContext[map[string]string]) (runtime.Service, error) {
			return testRuntimeService{info: runtime.ServiceInfo{Name: "api", Enabled: true}}, nil
		})
		return nil
	}))
	if err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}

	kind, ok := bootstrap.ServiceKindForType("api")
	if !ok || kind != runtime.ServiceKindHTTP {
		t.Fatalf("ServiceKindForType() = %q, %t, want http, true", kind, ok)
	}
}

func TestServiceBootstrapRuntimeCapabilitiesUseServiceLocalScope(t *testing.T) {
	base := map[string]string{"app": "demo"}
	bootstrap := runtime.NewServiceBootstrap[map[string]string](func(string) (map[string]runtime.ServiceSpec, error) {
		return nil, nil
	})
	bootstrap.RegisterServiceDefinition(runtime.ServiceDefinition[map[string]string]{
		Type: "api",
		RuntimeCapabilityFactory: func(ctx runtime.RuntimeCapabilityContext[map[string]string]) []runtime.CapabilityContract {
			return []runtime.CapabilityContract{
				runtime.NewCapability(ctx.LocalName("openai"), func(*runtime.App) error { return nil }),
			}
		},
		ContextFactory: func(runtime.ServiceContext[map[string]string]) (runtime.Service, error) {
			return testRuntimeService{info: runtime.ServiceInfo{Name: "api", Enabled: true}}, nil
		},
	})

	capabilities := bootstrap.RuntimeCapabilitiesForService(runtime.NewRuntimeCapabilityContext("config.yaml", "api", "api", "", base))
	if len(capabilities) != 1 {
		t.Fatalf("len(capabilities) = %d, want 1", len(capabilities))
	}
	metadata := capabilities[0].Metadata()
	if metadata.Name != "api.openai" || metadata.Group != "api" || metadata.Scope != runtime.ScopeServiceLocal {
		t.Fatalf("Metadata() = %+v", metadata)
	}
}

func TestServiceBootstrapRequiresLoader(t *testing.T) {
	_, err := new(runtime.ServiceBootstrap[map[string]string]).LoadServiceSpecs("config.yaml")
	if !errors.Is(err, runtime.ErrServiceSpecLoaderRequired) {
		t.Fatalf("LoadServiceSpecs() error = %v, want %v", err, runtime.ErrServiceSpecLoaderRequired)
	}
}

func TestServiceBootstrapBuildServiceWithCapabilities(t *testing.T) {
	bootstrap := runtime.NewServiceBootstrapWithResolver[map[string]string](
		func(string) (map[string]runtime.ServiceSpec, error) { return nil, nil },
		func(string, string) (runtime.ServiceSpec, error) {
			return runtime.ServiceSpec{ConfigKey: "admin_service"}, nil
		},
	)
	bootstrap.RegisterServiceDefinition(runtime.ServiceDefinition[map[string]string]{
		Type: "admin",
		Kind: runtime.ServiceKindHTTP,
		ContextFactory: func(ctx runtime.ServiceContext[map[string]string]) (runtime.Service, error) {
			if ctx.ConfigKey != "admin_service" {
				t.Fatalf("ConfigKey = %q, want admin_service", ctx.ConfigKey)
			}
			return testRuntimeService{info: runtime.ServiceInfo{Name: "admin", Type: "admin"}}, nil
		},
	})
	registry := runtime.NewLocalCapabilityRegistry()
	registry.AddName("admin.queue")
	registry.AddName("eventbus")

	svc, err := bootstrap.BuildServiceWithCapabilities("config.yaml", "admin", "admin", map[string]string{}, registry)
	if err != nil {
		t.Fatalf("BuildServiceWithCapabilities() error = %v", err)
	}
	if got, want := svc.ServiceInfo().Capabilities, []string{"queue", "eventbus"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Capabilities = %v, want %v", got, want)
	}
}
