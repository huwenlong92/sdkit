package runtime

import (
	"context"
	"errors"
)

var (
	ErrCapabilityNil           = errors.New("runtime: capability is nil")
	ErrCapabilityNameRequired  = errors.New("runtime: capability name is required")
	ErrCapabilityNameReserved  = errors.New("runtime: capability name is reserved")
	ErrCapabilityNameDuplicate = errors.New("runtime: capability name is duplicate")
	ErrCapabilityNotFound      = errors.New("runtime: capability not found")
)

type CapabilityContract interface {
	Name() string
	Metadata() CapabilityMetadata
	Dependencies() []Dependency
	Register(app *App) error
	Shutdown(ctx context.Context) error
}

type Capability interface {
	CapabilityContract
	Status() Status
}

type CapabilityFunc func(app *App) error

func (f CapabilityFunc) Name() string {
	return ""
}

func (f CapabilityFunc) Metadata() CapabilityMetadata {
	return CapabilityMetadata{}
}

func (f CapabilityFunc) Dependencies() []Dependency {
	return nil
}

func (f CapabilityFunc) Register(app *App) error {
	if f == nil {
		return ErrCapabilityNil
	}
	return f(app)
}

func (f CapabilityFunc) Shutdown(context.Context) error {
	return nil
}

func (f CapabilityFunc) Status() Status {
	return StatusStopped
}

func NewCapability(name string, register func(app *App) error) Capability {
	return NewCapabilityWithShutdown(name, register, nil)
}

func NewCapabilityWithShutdown(name string, register func(app *App) error, shutdown func(ctx context.Context) error) Capability {
	return NewCapabilityWithMetadata(CapabilityMetadata{Name: name}, register, shutdown)
}

func NewCapabilityWithMetadata(metadata CapabilityMetadata, register func(app *App) error, shutdown func(ctx context.Context) error) Capability {
	return NewCapabilityWithMetadataAndDependencies(metadata, nil, register, shutdown)
}

func NewCapabilityWithDependencies(name string, dependencies []Dependency, register func(app *App) error, shutdown func(ctx context.Context) error) Capability {
	return NewCapabilityWithMetadataAndDependencies(CapabilityMetadata{Name: name}, dependencies, register, shutdown)
}

func NewCapabilityWithMetadataAndDependencies(metadata CapabilityMetadata, dependencies []Dependency, register func(app *App) error, shutdown func(ctx context.Context) error) Capability {
	metadata.Scope = normalizeCapabilityScope(metadata.Scope)
	return namedCapabilityFunc{
		metadata:     metadata,
		dependencies: cloneDependencies(dependencies),
		register:     register,
		shutdown:     shutdown,
		state:        newRuntimeStatus(),
	}
}

type namedCapabilityFunc struct {
	metadata     CapabilityMetadata
	dependencies []Dependency
	register     func(app *App) error
	shutdown     func(ctx context.Context) error
	state        *runtimeStatus
}

func (f namedCapabilityFunc) Name() string {
	return f.metadata.Name
}

func (f namedCapabilityFunc) Metadata() CapabilityMetadata {
	return f.metadata
}

func (f namedCapabilityFunc) Dependencies() []Dependency {
	return cloneDependencies(f.dependencies)
}

func (f namedCapabilityFunc) Register(app *App) error {
	if f.register == nil {
		return ErrCapabilityNil
	}
	return f.register(app)
}

func (f namedCapabilityFunc) Shutdown(ctx context.Context) error {
	if f.shutdown == nil {
		return nil
	}
	return f.shutdown(ctx)
}

func (f namedCapabilityFunc) Status() Status {
	return f.state.Status()
}

func (f namedCapabilityFunc) setRuntimeStatus(status Status, err error) {
	f.state.Set(status, err)
}

func (f namedCapabilityFunc) runtimeHealth(name string) Health {
	return f.state.Health(name)
}

func isReservedCapabilityName(name string) bool {
	switch name {
	case "capability", "default", "main":
		return true
	default:
		return false
	}
}
