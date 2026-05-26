package tests

import (
	"errors"
	"reflect"
	"testing"

	coreruntime "github.com/huwenlong92/sdkit/core/runtime"
)

func TestServiceBootstrapBuildsServicesFromLoader(t *testing.T) {
	base := map[string]string{"app": "demo"}
	bootstrap := coreruntime.NewServiceBootstrap[map[string]string](func(configFile string) (map[string]coreruntime.ServiceSpec, error) {
		if configFile != "config.yaml" {
			t.Fatalf("configFile = %q, want config.yaml", configFile)
		}
		return map[string]coreruntime.ServiceSpec{
			"public": {
				Type:      "api",
				ConfigKey: "api_public",
			},
		}, nil
	})
	bootstrap.RegisterServiceDefinition(coreruntime.ServiceDefinition[map[string]string]{
		Type: "api",
		Kind: coreruntime.ServiceKindHTTP,
		ContextFactory: func(ctx coreruntime.ServiceContext[map[string]string]) (coreruntime.Service, error) {
			if ctx.ConfigFile != "config.yaml" || ctx.Name != "public" || ctx.Type != "api" || ctx.ConfigKey != "api_public" {
				t.Fatalf("ServiceContext = %+v", ctx)
			}
			if ctx.Base["app"] != "demo" {
				t.Fatalf("ServiceContext.Base = %v, want demo", ctx.Base)
			}
			return testRuntimeService{info: coreruntime.ServiceInfo{Name: "public", Enabled: true}}, nil
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
	if info.Type != "api" || info.Kind != coreruntime.ServiceKindHTTP {
		t.Fatalf("ServiceInfo() = %+v, want type api kind http", info)
	}
}

func TestServiceBootstrapUsesResolverForSingleServiceConfigKey(t *testing.T) {
	bootstrap := coreruntime.NewServiceBootstrapWithResolver[map[string]string](
		func(string) (map[string]coreruntime.ServiceSpec, error) {
			return nil, errors.New("load all specs should not run")
		},
		func(configFile string, name string) (coreruntime.ServiceSpec, error) {
			if configFile != "config.yaml" || name != "admin" {
				t.Fatalf("resolver got configFile=%q name=%q", configFile, name)
			}
			return coreruntime.ServiceSpec{ConfigKey: "admin_service"}, nil
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
	bootstrap := coreruntime.NewServiceBootstrap[map[string]string](func(string) (map[string]coreruntime.ServiceSpec, error) {
		return nil, nil
	})
	err := bootstrap.RegisterProvider(coreruntime.ServiceProviderFunc[map[string]string](func(app *coreruntime.ServiceApp[map[string]string]) error {
		app.Service("api").Kind(coreruntime.ServiceKindHTTP).FactoryContext(func(coreruntime.ServiceContext[map[string]string]) (coreruntime.Service, error) {
			return testRuntimeService{info: coreruntime.ServiceInfo{Name: "api", Enabled: true}}, nil
		})
		return nil
	}))
	if err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}

	kind, ok := bootstrap.ServiceKindForType("api")
	if !ok || kind != coreruntime.ServiceKindHTTP {
		t.Fatalf("ServiceKindForType() = %q, %t, want http, true", kind, ok)
	}
}

func TestServiceBootstrapRuntimeCapabilitiesUseServiceLocalScope(t *testing.T) {
	base := map[string]string{"app": "demo"}
	bootstrap := coreruntime.NewServiceBootstrap[map[string]string](func(string) (map[string]coreruntime.ServiceSpec, error) {
		return nil, nil
	})
	bootstrap.RegisterServiceDefinition(coreruntime.ServiceDefinition[map[string]string]{
		Type: "api",
		RuntimeCapabilityFactory: func(ctx coreruntime.RuntimeCapabilityContext[map[string]string]) []coreruntime.CapabilityContract {
			return []coreruntime.CapabilityContract{
				coreruntime.NewCapability(ctx.LocalName("openai"), func(*coreruntime.App) error { return nil }),
			}
		},
		ContextFactory: func(coreruntime.ServiceContext[map[string]string]) (coreruntime.Service, error) {
			return testRuntimeService{info: coreruntime.ServiceInfo{Name: "api", Enabled: true}}, nil
		},
	})

	capabilities := bootstrap.RuntimeCapabilitiesForService(coreruntime.NewRuntimeCapabilityContext("config.yaml", "api", "api", "", base))
	if len(capabilities) != 1 {
		t.Fatalf("len(capabilities) = %d, want 1", len(capabilities))
	}
	metadata := capabilities[0].Metadata()
	if metadata.Name != "api.openai" || metadata.Group != "api" || metadata.Scope != coreruntime.ScopeServiceLocal {
		t.Fatalf("Metadata() = %+v", metadata)
	}
}

func TestServiceBootstrapRequiresLoader(t *testing.T) {
	_, err := new(coreruntime.ServiceBootstrap[map[string]string]).LoadServiceSpecs("config.yaml")
	if !errors.Is(err, coreruntime.ErrServiceSpecLoaderRequired) {
		t.Fatalf("LoadServiceSpecs() error = %v, want %v", err, coreruntime.ErrServiceSpecLoaderRequired)
	}
}

func TestServiceBootstrapBuildServiceWithCapabilities(t *testing.T) {
	bootstrap := coreruntime.NewServiceBootstrapWithResolver[map[string]string](
		func(string) (map[string]coreruntime.ServiceSpec, error) { return nil, nil },
		func(string, string) (coreruntime.ServiceSpec, error) {
			return coreruntime.ServiceSpec{ConfigKey: "admin_service"}, nil
		},
	)
	bootstrap.RegisterServiceDefinition(coreruntime.ServiceDefinition[map[string]string]{
		Type: "admin",
		Kind: coreruntime.ServiceKindHTTP,
		ContextFactory: func(ctx coreruntime.ServiceContext[map[string]string]) (coreruntime.Service, error) {
			if ctx.ConfigKey != "admin_service" {
				t.Fatalf("ConfigKey = %q, want admin_service", ctx.ConfigKey)
			}
			return testRuntimeService{info: coreruntime.ServiceInfo{Name: "admin", Type: "admin"}}, nil
		},
	})
	registry := coreruntime.NewLocalCapabilityRegistry()
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
