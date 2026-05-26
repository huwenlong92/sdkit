package database

import (
	coredatabase "github.com/huwenlong92/sdkit/core/database"
	"github.com/huwenlong92/sdkit/core/runtime"
)

const Name = string(coredatabase.KeyDatabase)

type Config = coredatabase.Config
type Database = coredatabase.Database

type ConfigLoader func(app *runtime.App) (Config, error)
type ModeLoader func(app *runtime.App) (string, error)
