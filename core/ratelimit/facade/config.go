package ratelimit

import (
	coreratelimit "github.com/huwenlong92/sdkit/core/ratelimit"
	"github.com/huwenlong92/sdkit/core/ratelimit/store"
	"github.com/huwenlong92/sdkit/core/runtime"
)

const KeyRateLimit runtime.Key = "ratelimit"

type Config = coreratelimit.Config
type BBRConfig = coreratelimit.BBRConfig
type Store = store.Store

type ConfigLoader func(app *runtime.App) (Config, error)
