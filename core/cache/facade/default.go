package cache

import (
	corecache "github.com/huwenlong92/sdkit/core/cache"
	"github.com/huwenlong92/sdkit/core/runtime"
)

func Default() Cache {
	return corecache.Default()
}

func From(app *runtime.App) Cache {
	return corecache.From(app)
}
