package ratelimit

import (
	rlMiddleware "github.com/huwenlong92/sdkit/core/ratelimit/middleware"
	"github.com/huwenlong92/sdkit/core/runtime"
)

func From(app *runtime.App) Store {
	if app != nil {
		if value, ok := app.Container().Get(KeyRateLimit); ok {
			if store, ok := value.(Store); ok {
				return store
			}
		}
	}
	return rlMiddleware.CustomStore
}
