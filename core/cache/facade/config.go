package cache

import (
	corecache "github.com/huwenlong92/sdkit/core/cache"
	"github.com/huwenlong92/sdkit/core/runtime"
)

const KeyCache = corecache.KeyCache

type Config = corecache.Config
type Cache = corecache.Cache
type Option = corecache.Option

type ConfigLoader func(app *runtime.App) (Config, error)
