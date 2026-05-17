package cache

import (
	"github.com/huwenlong92/sdkit/core/runtime"
)

const KeyCache runtime.Key = "cache"

func From(app *runtime.App) Cache {
	if app != nil {
		if value, ok := app.Container().Get(KeyCache); ok {
			if c, ok := value.(Cache); ok {
				return c
			}
		}
	}
	return Default()
}

func Bind(app *runtime.App, c Cache) error {
	if c == nil {
		defaultCache = nil
		if app == nil {
			return nil
		}
		return runtime.ErrContainerValueNil
	}
	defaultCache = c
	if app == nil {
		return nil
	}
	return app.Container().Bind(KeyCache, c)
}
