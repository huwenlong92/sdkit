package email

import (
	coreemail "github.com/huwenlong92/sdkit/core/email"
	"github.com/huwenlong92/sdkit/core/runtime"
)

type Config = coreemail.Config
type ProviderConfig = coreemail.ProviderConfig
type Manager = coreemail.Manager
type Message = coreemail.Message
type SendResult = coreemail.SendResult
type Middleware = coreemail.Middleware

const (
	KeyEmail = coreemail.KeyEmail
	Name     = string(KeyEmail)
)

func FromDefault() (*Manager, error) {
	return coreemail.ManagerDefault()
}

func From(app *runtime.App) *Manager {
	return coreemail.From(app)
}
