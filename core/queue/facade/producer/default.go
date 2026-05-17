package producer

import (
	corequeue "github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/runtime"
)

type ServiceContext interface {
	CapabilityLocalFirst(name string) (any, bool)
}

func From(app *runtime.App) Client {
	if app == nil {
		return nil
	}
	value, ok := app.Container().Get(KeyQueue)
	if !ok {
		return nil
	}
	client, _ := value.(corequeue.Client)
	return client
}

func ClientFrom(app *runtime.App) Client {
	return From(app)
}

func FromServiceContext(ctx ServiceContext) (Client, bool) {
	if ctx == nil {
		return nil, false
	}
	value, ok := ctx.CapabilityLocalFirst(Name)
	if !ok {
		return nil, false
	}
	client, ok := value.(corequeue.Client)
	if !ok || client == nil {
		return nil, false
	}
	return client, true
}

func RuntimeFromServiceContext(ctx ServiceContext) *corequeue.RuntimeInstance {
	client, ok := FromServiceContext(ctx)
	if !ok {
		return nil
	}
	return corequeue.NewRuntimeInstanceFromParts(corequeue.RuntimeParts{Client: client})
}
