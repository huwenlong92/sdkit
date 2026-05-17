package redis

import (
	coreredis "github.com/huwenlong92/sdkit/core/redis"
	"github.com/huwenlong92/sdkit/core/runtime"
)

const KeyRedis = coreredis.KeyRedis

type Config = coreredis.Config
type RuntimeClient = coreredis.RuntimeClient

type ConfigLoader func(app *runtime.App) (Config, error)
