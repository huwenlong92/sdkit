package casbin

import (
	corecasbin "github.com/huwenlong92/sdkit/core/casbin"
	"github.com/huwenlong92/sdkit/core/database"
	"github.com/huwenlong92/sdkit/core/runtime"
)

const Name = string(corecasbin.KeyCasbin)

type Config = corecasbin.Config
type Manager = corecasbin.Manager
type Database = database.Database

type ConfigLoader func(app *runtime.App) (Config, error)
type DatabaseLoader func(app *runtime.App) (*Database, error)
