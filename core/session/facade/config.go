package session

import (
	"github.com/huwenlong92/sdkit/core/runtime"
	coresession "github.com/huwenlong92/sdkit/core/session"
)

const (
	KeySession runtime.Key = "session"
	Name                   = string(KeySession)
)

type Config = coresession.Config
type Session = coresession.Session
type Store = coresession.Store

type ConfigLoader func(app *runtime.App) (Config, error)
