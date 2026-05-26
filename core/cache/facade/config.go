package cache

import (
	corecache "github.com/huwenlong92/sdkit/core/cache"
	"github.com/huwenlong92/sdkit/core/runtime"
)

const Name = string(corecache.KeyCache)

type Config = corecache.Config
type Cache = corecache.Cache

type ConfigLoader func(app *runtime.App) (Config, error)
