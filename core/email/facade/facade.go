package email

import (
	"io/fs"

	coreemail "github.com/huwenlong92/sdkit/core/email"
	"github.com/huwenlong92/sdkit/core/runtime"
)

type Config = coreemail.Config
type ProviderConfig = coreemail.ProviderConfig
type Manager = coreemail.Manager
type Message = coreemail.Message
type DirectMessage = coreemail.DirectMessage
type TemplateMessage = coreemail.TemplateMessage
type Template = coreemail.Template
type TemplateRenderer = coreemail.TemplateRenderer
type SendResult = coreemail.SendResult
type Middleware = coreemail.Middleware

const Name = string(coreemail.KeyEmail)

func FromDefault() (*Manager, error) {
	return coreemail.ManagerDefault()
}

func From(app *runtime.App) *Manager {
	return coreemail.From(app)
}

func LoadTemplates(fsys fs.FS, templates map[string]Template) (map[string]Template, error) {
	return coreemail.LoadTemplates(fsys, templates)
}
