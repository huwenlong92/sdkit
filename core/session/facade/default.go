package session

import (
	"github.com/huwenlong92/sdkit/core/runtime"
	coresession "github.com/huwenlong92/sdkit/core/session"

	"github.com/gin-gonic/gin"
)

type ServiceContext interface {
	CapabilityLocalFirst(name string) (any, bool)
}

func GetStore() Store {
	return coresession.GetStore()
}

func From(app *runtime.App) Store {
	if app != nil {
		if value, ok := app.Container().Get(KeySession); ok {
			if store, ok := value.(Store); ok {
				return store
			}
		}
	}
	return coresession.GetStore()
}

func FromServiceContext(ctx ServiceContext) (Store, bool) {
	if ctx == nil {
		return nil, false
	}
	value, ok := ctx.CapabilityLocalFirst(Name)
	if !ok {
		return nil, false
	}
	store, ok := value.(Store)
	if !ok || store == nil {
		return nil, false
	}
	return store, true
}

func MiddlewareFromServiceContext(ctx ServiceContext) gin.HandlerFunc {
	store, ok := FromServiceContext(ctx)
	if !ok {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	return coresession.WithStore(store)
}
