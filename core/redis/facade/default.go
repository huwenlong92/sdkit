package redis

import (
	coreredis "github.com/huwenlong92/sdkit/core/redis"
	"github.com/huwenlong92/sdkit/core/runtime"
)

func From(app *runtime.App) *RuntimeClient {
	return coreredis.From(app)
}
