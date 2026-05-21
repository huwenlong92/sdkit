package ratelimit

import (
	"github.com/huwenlong92/sdkit/core/runtime"
	"github.com/huwenlong92/sdkit/pkg/ratelimit"
	"github.com/huwenlong92/sdkit/pkg/ratelimit/store"
)

const KeyRateLimit runtime.Key = "ratelimit"

type Config = ratelimit.Config
type BBRConfig = ratelimit.BBRConfig
type Store = store.Store

type ConfigLoader func(app *runtime.App) (Config, error)
