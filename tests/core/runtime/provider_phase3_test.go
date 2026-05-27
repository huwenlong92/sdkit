package tests

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/huwenlong92/sdkit/core/runtime"
)

const keyPhase3Shared runtime.Key = "phase3-shared"

type phase3SharedResource struct {
	name string
}

type phase3Provider struct {
	name              string
	metadata          runtime.ProviderMetadata
	app               *runtime.App
	registerResources *[]*phase3SharedResource
	startResources    *[]*phase3SharedResource
}

func (p *phase3Provider) Name() string {
	return p.name
}

func (p *phase3Provider) Metadata() runtime.ProviderMetadata {
	metadata := p.metadata
	if metadata.Name == "" {
		metadata.Name = p.name
	}
	return metadata
}

func (p *phase3Provider) Dependencies() []runtime.Dependency {
	return nil
}

func (p *phase3Provider) Register(app *runtime.App) error {
	p.app = app
	resource, _ := app.Container().MustGet(keyPhase3Shared).(*phase3SharedResource)
	*p.registerResources = append(*p.registerResources, resource)
	return nil
}

func (p *phase3Provider) Start(context.Context) error {
	resource, _ := p.app.Container().MustGet(keyPhase3Shared).(*phase3SharedResource)
	*p.startResources = append(*p.startResources, resource)
	return nil
}

func (p *phase3Provider) Stop(context.Context) error {
	return nil
}

func TestPhase3RegisterValidatesProviderNames(t *testing.T) {
	t.Run("name required", func(t *testing.T) {
		app := runtime.New()
		if err := app.Register(testProvider{}); !errors.Is(err, runtime.ErrProviderNameRequired) {
			t.Fatalf("Register(empty name) error = %v, want ErrProviderNameRequired", err)
		}
	})

	for _, name := range []string{"provider", "default", "main"} {
		t.Run("reserved "+name, func(t *testing.T) {
			app := runtime.New()
			if err := app.Register(testProvider{name: name}); !errors.Is(err, runtime.ErrProviderNameReserved) {
				t.Fatalf("Register(%q) error = %v, want ErrProviderNameReserved", name, err)
			}
		})
	}

	t.Run("duplicate existing", func(t *testing.T) {
		app := runtime.New()
		if err := app.Register(testProvider{name: "api"}); err != nil {
			t.Fatalf("Register(api) error = %v", err)
		}
		if err := app.Register(testProvider{name: "api"}); !errors.Is(err, runtime.ErrProviderNameDuplicate) {
			t.Fatalf("Register(duplicate api) error = %v, want ErrProviderNameDuplicate", err)
		}
		if got := len(app.Providers()); got != 1 {
			t.Fatalf("provider count = %d, want 1", got)
		}
	})

	t.Run("duplicate in batch does not partially register", func(t *testing.T) {
		app := runtime.New()
		err := app.Register(
			testProvider{name: "api"},
			testProvider{name: "api"},
		)
		if !errors.Is(err, runtime.ErrProviderNameDuplicate) {
			t.Fatalf("Register(duplicate batch) error = %v, want ErrProviderNameDuplicate", err)
		}
		if got := len(app.Providers()); got != 0 {
			t.Fatalf("provider count = %d, want 0", got)
		}
	})
}

func TestPhase3AppRegisterOnlyStoresProviders(t *testing.T) {
	app := runtime.New()
	var calls []string
	provider := testProvider{
		name: "api",
		register: func(*runtime.App) error {
			calls = append(calls, "register")
			return nil
		},
		start: func(context.Context) error {
			calls = append(calls, "start")
			return nil
		},
	}

	if err := app.Register(provider); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("calls after Register() = %v, want none", calls)
	}
	if names := providerNames(app.Providers()); !reflect.DeepEqual(names, []string{"api"}) {
		t.Fatalf("Providers() = %v, want [api]", names)
	}
}

func TestPhase3RunRegistersAllProvidersBeforeStart(t *testing.T) {
	app := runtime.New()
	registered := map[string]bool{}
	var calls []string

	if err := app.Register(
		testProvider{
			name: "api",
			register: func(*runtime.App) error {
				registered["api"] = true
				calls = append(calls, "api.register")
				return nil
			},
			start: func(context.Context) error {
				if !registered["worker"] {
					t.Fatal("api.Start() ran before worker.Register()")
				}
				calls = append(calls, "api.start")
				return nil
			},
		},
		testProvider{
			name: "worker",
			register: func(*runtime.App) error {
				registered["worker"] = true
				calls = append(calls, "worker.register")
				return nil
			},
			start: func(context.Context) error {
				calls = append(calls, "worker.start")
				return nil
			},
		},
	); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := []string{"api.register", "worker.register", "api.start", "worker.start"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestPhase3RunDeduplicatesProviderRuntimeCapabilitiesByName(t *testing.T) {
	app := runtime.New()
	registerCount := 0
	shared := runtime.NewCapability("eventbus", func(*runtime.App) error {
		registerCount++
		return nil
	})

	if err := app.Register(
		testProvider{name: "worker", runtimeCapabilities: []runtime.CapabilityContract{shared}},
		testProvider{name: "crontab", runtimeCapabilities: []runtime.CapabilityContract{shared}},
	); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.ValidateDependencies(); err != nil {
		t.Fatalf("ValidateDependencies() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if registerCount != 1 {
		t.Fatalf("eventbus register count = %d, want 1", registerCount)
	}
}

func TestPhase3StartRollbackStopsStartedProvidersInReverseOrder(t *testing.T) {
	app := runtime.New()
	startErr := errors.New("worker failed")
	var calls []string

	if err := app.Register(
		testProvider{
			name: "api",
			register: func(*runtime.App) error {
				calls = append(calls, "api.register")
				return nil
			},
			start: func(context.Context) error {
				calls = append(calls, "api.start")
				return nil
			},
			stop: func(context.Context) error {
				calls = append(calls, "api.stop")
				return nil
			},
		},
		testProvider{
			name: "admin",
			register: func(*runtime.App) error {
				calls = append(calls, "admin.register")
				return nil
			},
			start: func(context.Context) error {
				calls = append(calls, "admin.start")
				return nil
			},
			stop: func(context.Context) error {
				calls = append(calls, "admin.stop")
				return nil
			},
		},
		testProvider{
			name: "worker",
			register: func(*runtime.App) error {
				calls = append(calls, "worker.register")
				return nil
			},
			start: func(context.Context) error {
				calls = append(calls, "worker.start")
				return startErr
			},
			stop: func(context.Context) error {
				calls = append(calls, "worker.stop")
				return nil
			},
		},
	); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.Run(context.Background()); !errors.Is(err, startErr) {
		t.Fatalf("Run() error = %v, want startErr", err)
	}
	want := []string{
		"api.register",
		"admin.register",
		"worker.register",
		"api.start",
		"admin.start",
		"worker.start",
		"worker.stop",
		"admin.stop",
		"api.stop",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestPhase3ServeModeProvidersShareCapability(t *testing.T) {
	app := runtime.New()
	resource := &phase3SharedResource{name: "shared"}
	var registerResources []*phase3SharedResource
	var startResources []*phase3SharedResource

	if err := app.Use(runtime.NewCapability("shared", func(app *runtime.App) error {
		return app.Container().Bind(keyPhase3Shared, resource)
	})); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Register(
		&phase3Provider{name: "api", registerResources: &registerResources, startResources: &startResources},
		&phase3Provider{name: "worker", registerResources: &registerResources, startResources: &startResources},
	); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := len(registerResources); got != 2 {
		t.Fatalf("register resource count = %d, want 2", got)
	}
	if got := len(startResources); got != 2 {
		t.Fatalf("start resource count = %d, want 2", got)
	}
	for i, got := range append(registerResources, startResources...) {
		if got != resource {
			t.Fatalf("resource[%d] = %p, want %p", i, got, resource)
		}
	}
}

func providerNames(providers []runtime.Provider) []string {
	names := make([]string, 0, len(providers))
	for _, provider := range providers {
		names = append(names, provider.Name())
	}
	return names
}
