package operations

import (
	corequeue "github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/runtime"
)

type ServiceContext interface {
	CapabilityLocalFirst(name string) (any, bool)
}

func From(app *runtime.App) *corequeue.RuntimeInstance {
	if app == nil {
		return nil
	}
	value, ok := app.Container().Get(KeyQueue)
	if !ok {
		return nil
	}
	runtime, _ := value.(*corequeue.RuntimeInstance)
	return runtime
}

func RuntimeFrom(app *runtime.App) *corequeue.RuntimeInstance {
	return From(app)
}

func FromServiceContext(ctx ServiceContext) (*corequeue.RuntimeInstance, bool) {
	if ctx == nil {
		return nil, false
	}
	value, ok := ctx.CapabilityLocalFirst(Name)
	if !ok {
		return nil, false
	}
	runtime, ok := value.(*corequeue.RuntimeInstance)
	if !ok || runtime == nil {
		return nil, false
	}
	return runtime, true
}

func RuntimeFromServiceContext(ctx ServiceContext) *corequeue.RuntimeInstance {
	runtime, _ := FromServiceContext(ctx)
	return runtime
}

func OperationsFromServiceContext(ctx ServiceContext) (*corequeue.OperationsRuntime, bool) {
	runtime, ok := FromServiceContext(ctx)
	if !ok || runtime.Operations() == nil {
		return nil, false
	}
	return runtime.Operations(), true
}
