package tests

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/runtime"
)

const (
	keyDatabase runtime.Key = "database"
	keyLogger   runtime.Key = "logger"
)

type testCapability struct {
	name         string
	metadata     runtime.CapabilityMetadata
	dependencies []runtime.Dependency
	register     func(app *runtime.App) error
	shutdown     func(ctx context.Context) error
}

func (c testCapability) Name() string {
	return c.name
}

func (c testCapability) Metadata() runtime.CapabilityMetadata {
	metadata := c.metadata
	if metadata.Name == "" {
		metadata.Name = c.name
	}
	return metadata
}

func (c testCapability) Dependencies() []runtime.Dependency {
	return c.dependencies
}

func (c testCapability) Register(app *runtime.App) error {
	return c.register(app)
}

func (c testCapability) Shutdown(ctx context.Context) error {
	if c.shutdown == nil {
		return nil
	}
	return c.shutdown(ctx)
}

type testProvider struct {
	name                string
	metadata            runtime.ProviderMetadata
	dependencies        []runtime.Dependency
	runtimeCapabilities []runtime.CapabilityContract
	register            func(app *runtime.App) error
	start               func(ctx context.Context) error
	stop                func(ctx context.Context) error
}

func (p testProvider) Name() string {
	return p.name
}

func (p testProvider) Metadata() runtime.ProviderMetadata {
	metadata := p.metadata
	if metadata.Name == "" {
		metadata.Name = p.name
	}
	return metadata
}

func (p testProvider) Dependencies() []runtime.Dependency {
	return p.dependencies
}

func (p testProvider) RuntimeCapabilities() []runtime.CapabilityContract {
	return p.runtimeCapabilities
}

func (p testProvider) Register(app *runtime.App) error {
	if p.register == nil {
		return nil
	}
	return p.register(app)
}

func (p testProvider) Start(ctx context.Context) error {
	if p.start == nil {
		return nil
	}
	return p.start(ctx)
}

func (p testProvider) Stop(ctx context.Context) error {
	if p.stop == nil {
		return nil
	}
	return p.stop(ctx)
}

type testCommand struct {
	name     string
	metadata runtime.CommandMetadata
}

func (c testCommand) Name() string {
	return c.name
}

func (c testCommand) Metadata() runtime.CommandMetadata {
	metadata := c.metadata
	if metadata.Name == "" {
		metadata.Name = c.name
	}
	return metadata
}

func (c testCommand) Run(context.Context, *runtime.App, []string) error {
	return nil
}

func TestContainerBindGetMustGet(t *testing.T) {
	container := runtime.NewContainer()
	if err := container.Bind(keyLogger, "default"); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if got, ok := container.Get(keyLogger); !ok || got != "default" {
		t.Fatalf("Get() = %v, %v; want default, true", got, ok)
	}
	if got := container.MustGet(keyLogger); got != "default" {
		t.Fatalf("MustGet() = %v, want default", got)
	}
	if got := container.MustGet(runtime.Key("missing")); got != nil {
		t.Fatalf("MustGet(missing) = %v, want nil", got)
	}
}

func TestContainerBindValidation(t *testing.T) {
	container := runtime.NewContainer()
	var nilContainer *runtime.Container
	if !errors.Is(nilContainer.Bind(runtime.Key("key"), "value"), runtime.ErrContainerNil) {
		t.Fatalf("Bind(nil container) must return ErrContainerNil")
	}
	if !errors.Is(container.Bind(runtime.Key(""), "value"), runtime.ErrContainerKeyRequired) {
		t.Fatalf("Bind(empty key) must return ErrContainerKeyRequired")
	}
	if !errors.Is(container.Bind(runtime.Key("nil"), nil), runtime.ErrContainerValueNil) {
		t.Fatalf("Bind(nil value) must return ErrContainerValueNil")
	}
}

func TestUseOnlyStoresCapabilityAndRunRegistersIt(t *testing.T) {
	app := &runtime.App{}
	capability := testCapability{
		name: "logger",
		register: func(app *runtime.App) error {
			return app.Container().Bind(keyLogger, "default")
		},
	}

	if err := app.Use(capability); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if got := app.Container().MustGet(keyLogger); got != nil {
		t.Fatalf("container logger after Use() = %v, want nil", got)
	}
	if names := capabilityNames(app.Capabilities()); !reflect.DeepEqual(names, []string{"logger"}) {
		t.Fatalf("Capabilities() = %v, want [logger]", names)
	}
	if err := app.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := app.Container().MustGet(keyLogger); got != "default" {
		t.Fatalf("container logger after Run() = %v, want default", got)
	}
}

func TestNewCapability(t *testing.T) {
	app := runtime.New()
	capability := runtime.NewCapability("database", func(app *runtime.App) error {
		return app.Container().Bind(keyDatabase, "primary")
	})

	if capability.Name() != "database" {
		t.Fatalf("capability name = %q, want database", capability.Name())
	}
	if err := app.Use(capability); err != nil {
		t.Fatalf("Use(NewCapability) error = %v", err)
	}
	if err := app.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := app.Container().MustGet(keyDatabase); got != "primary" {
		t.Fatalf("container database = %v, want primary", got)
	}
}

func TestRegisterCommandOnlyStoresCommand(t *testing.T) {
	app := runtime.New()
	if err := app.RegisterCommand(testCommand{name: "serve"}); err != nil {
		t.Fatalf("RegisterCommand() error = %v", err)
	}
	if names := commandNames(app.Commands()); !reflect.DeepEqual(names, []string{"serve"}) {
		t.Fatalf("Commands() = %v, want [serve]", names)
	}
}

func TestRunRegistersAndStartsProviders(t *testing.T) {
	app := runtime.New()
	if err := app.Use(runtime.NewCapability("database", func(app *runtime.App) error {
		return app.Container().Bind(keyDatabase, "primary")
	})); err != nil {
		t.Fatalf("Use() error = %v", err)
	}

	var calls []string
	provider := testProvider{
		name: "api",
		register: func(app *runtime.App) error {
			if got := app.Container().MustGet(keyDatabase); got != "primary" {
				t.Fatalf("provider register database = %v, want primary", got)
			}
			calls = append(calls, "register")
			return nil
		},
		start: func(ctx context.Context) error {
			if got := logger.Field(ctx, logger.TraceIDKey); got != "trace-1" {
				t.Fatalf("provider start trace_id = %v, want trace-1", got)
			}
			calls = append(calls, "start")
			return nil
		},
	}
	if err := app.Register(provider); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	ctx := logger.WithField(context.Background(), logger.TraceIDKey, "trace-1")
	if err := app.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"register", "start"}) {
		t.Fatalf("provider calls = %v, want [register start]", calls)
	}
	if logger.Field(app.Context(), logger.TraceIDKey) != "trace-1" {
		t.Fatalf("app context trace_id = %v, want trace-1", logger.Field(app.Context(), logger.TraceIDKey))
	}
}

func TestRunRollsBackStartedProviders(t *testing.T) {
	app := runtime.New()
	startErr := errors.New("start failed")
	var calls []string
	if err := app.Register(
		testProvider{
			name: "provider1",
			start: func(context.Context) error {
				calls = append(calls, "provider1.start")
				return nil
			},
			stop: func(context.Context) error {
				calls = append(calls, "provider1.stop")
				return nil
			},
		},
		testProvider{
			name: "provider2",
			start: func(context.Context) error {
				calls = append(calls, "provider2.start")
				return nil
			},
			stop: func(context.Context) error {
				calls = append(calls, "provider2.stop")
				return nil
			},
		},
		testProvider{
			name: "provider3",
			start: func(context.Context) error {
				calls = append(calls, "provider3.start")
				return startErr
			},
			stop: func(context.Context) error {
				calls = append(calls, "provider3.stop")
				return nil
			},
		},
	); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := app.Run(); !errors.Is(err, startErr) {
		t.Fatalf("Run() error = %v, want startErr", err)
	}
	want := []string{
		"provider1.start",
		"provider2.start",
		"provider3.start",
		"provider3.stop",
		"provider2.stop",
		"provider1.stop",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("provider calls = %v, want %v", calls, want)
	}
}

func TestAppContextDefaultsToBackground(t *testing.T) {
	app := runtime.New()
	if app.Context() == nil {
		t.Fatalf("Context() must not return nil")
	}
}

func TestNilContractsReturnErrors(t *testing.T) {
	app := runtime.New()
	if !errors.Is(app.Use(nil), runtime.ErrCapabilityNil) {
		t.Fatalf("Use(nil) must return ErrCapabilityNil")
	}
	if !errors.Is(app.Register(nil), runtime.ErrProviderNil) {
		t.Fatalf("Register(nil) must return ErrProviderNil")
	}
	if !errors.Is(app.RegisterCommand(nil), runtime.ErrCommandNil) {
		t.Fatalf("RegisterCommand(nil) must return ErrCommandNil")
	}
	if !errors.Is(app.Use(runtime.CapabilityFunc(func(*runtime.App) error { return nil })), runtime.ErrCapabilityNameRequired) {
		t.Fatalf("Use(unnamed capability) must return ErrCapabilityNameRequired")
	}
}

func capabilityNames(capabilities []runtime.Capability) []string {
	names := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		names = append(names, capability.Name())
	}
	return names
}

func commandNames(commands []runtime.Command) []string {
	names := make([]string, 0, len(commands))
	for _, command := range commands {
		names = append(names, command.Name())
	}
	return names
}
