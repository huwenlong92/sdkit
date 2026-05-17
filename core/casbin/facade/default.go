package casbin

import (
	corecasbin "github.com/huwenlong92/sdkit/core/casbin"
	"github.com/huwenlong92/sdkit/core/runtime"
)

func Default() *Manager {
	return corecasbin.Default
}

func From(app *runtime.App) *Manager {
	return corecasbin.From(app)
}
