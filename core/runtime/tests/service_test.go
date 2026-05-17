package tests

import (
	"context"
	"errors"
	"reflect"
	"testing"

	coreruntime "github.com/huwenlong92/sdkit/core/runtime"
)

type testLocalCapability struct {
	name  string
	close func() error
}

func (c testLocalCapability) Name() string {
	return c.name
}

func (c testLocalCapability) Close() error {
	if c.close == nil {
		return nil
	}
	return c.close()
}

type testRuntimeService struct {
	info coreruntime.ServiceInfo
}

func (s testRuntimeService) ServiceInfo() coreruntime.ServiceInfo {
	return s.info
}

func (s testRuntimeService) Start(context.Context) error {
	return nil
}

func (s testRuntimeService) Shutdown(context.Context) error {
	return nil
}

func TestLocalCapabilityRegistryStoresInstancesAndClosesReverseOrder(t *testing.T) {
	registry := coreruntime.NewLocalCapabilityRegistry()
	var closed []string
	registry.Set("api.first", testLocalCapability{name: "api.first", close: func() error {
		closed = append(closed, "first")
		return nil
	}})
	registry.Set("api.second", testLocalCapability{name: "api.second", close: func() error {
		closed = append(closed, "second")
		return nil
	}})

	if got, ok := coreruntime.LocalCapabilityAs[testLocalCapability](registry, "api.first"); !ok || got.name != "api.first" {
		t.Fatalf("LocalCapabilityAs() = %#v, %t, want api.first, true", got, ok)
	}
	if names := registry.Names(); !reflect.DeepEqual(names, []string{"api.first", "api.second"}) {
		t.Fatalf("Names() = %v, want [api.first api.second]", names)
	}
	if err := registry.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !reflect.DeepEqual(closed, []string{"second", "first"}) {
		t.Fatalf("closed = %v, want [second first]", closed)
	}
}

func TestServiceRegistryBuildServicesAndPassesContext(t *testing.T) {
	base := map[string]string{"app": "demo"}
	registry := coreruntime.NewServiceRegistry[map[string]string]()
	registry.RegisterServiceDefinition(coreruntime.ServiceDefinition[map[string]string]{
		Type: "api",
		Kind: coreruntime.ServiceKindHTTP,
		ContextFactory: func(ctx coreruntime.ServiceContext[map[string]string]) (coreruntime.Service, error) {
			if ctx.ConfigFile != "config.yaml" || ctx.Name != "public" || ctx.Type != "api" || ctx.ConfigKey != "api_public" {
				t.Fatalf("ServiceContext = %+v", ctx)
			}
			if ctx.Base["app"] != "demo" {
				t.Fatalf("ServiceContext.Base = %v, want demo", ctx.Base)
			}
			ctx.Capabilities.Set("public.local", "local")
			return testRuntimeService{info: coreruntime.ServiceInfo{Name: "public", Enabled: true}}, nil
		},
	})

	services, err := registry.BuildServices("config.yaml", map[string]coreruntime.ServiceSpec{
		"public": {
			Type:      "api",
			ConfigKey: "api_public",
		},
	}, base)
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
	if !reflect.DeepEqual(info.Capabilities, []string{"local"}) {
		t.Fatalf("ServiceInfo().Capabilities = %v, want [local]", info.Capabilities)
	}
}

func TestServiceRegistryRuntimeCapabilitiesUseServiceLocalScope(t *testing.T) {
	base := map[string]string{"app": "demo"}
	registry := coreruntime.NewServiceRegistry[map[string]string]()
	registry.RegisterServiceDefinition(coreruntime.ServiceDefinition[map[string]string]{
		Type: "api",
		RuntimeCapabilityFactory: func(ctx coreruntime.RuntimeCapabilityContext[map[string]string]) []coreruntime.CapabilityContract {
			if ctx.BaseConfig()["app"] != "demo" {
				t.Fatalf("BaseConfig() = %v, want demo", ctx.BaseConfig())
			}
			return []coreruntime.CapabilityContract{
				coreruntime.NewCapability(ctx.LocalName("openai"), func(*coreruntime.App) error { return nil }),
			}
		},
		ContextFactory: func(coreruntime.ServiceContext[map[string]string]) (coreruntime.Service, error) {
			return testRuntimeService{info: coreruntime.ServiceInfo{Name: "api", Enabled: true}}, nil
		},
	})

	capabilities := registry.RuntimeCapabilitiesForService(coreruntime.NewRuntimeCapabilityContext("config.yaml", "api", "api", "", base))
	if len(capabilities) != 1 {
		t.Fatalf("len(capabilities) = %d, want 1", len(capabilities))
	}
	metadata := capabilities[0].Metadata()
	if metadata.Name != "api.openai" || metadata.Group != "api" || metadata.Scope != coreruntime.ScopeServiceLocal {
		t.Fatalf("Metadata() = %+v", metadata)
	}
}

func TestLocalCapabilityRegistryCloseJoinsErrors(t *testing.T) {
	firstErr := errors.New("first")
	secondErr := errors.New("second")
	registry := coreruntime.NewLocalCapabilityRegistry()
	registry.Set("first", testLocalCapability{name: "first", close: func() error { return firstErr }})
	registry.Set("second", testLocalCapability{name: "second", close: func() error { return secondErr }})

	err := registry.Close()
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("Close() error = %v, want both errors", err)
	}
}
