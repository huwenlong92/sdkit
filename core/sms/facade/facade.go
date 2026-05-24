package sms

import (
	"github.com/huwenlong92/sdkit/core/runtime"
	coresms "github.com/huwenlong92/sdkit/core/sms"
)

type Config = coresms.Config
type ProviderConfig = coresms.ProviderConfig
type Manager = coresms.Manager
type Message = coresms.Message
type TemplateMessage = coresms.TemplateMessage
type Param = coresms.Param
type SendResult = coresms.SendResult
type Middleware = coresms.Middleware
type RateLimiter = coresms.RateLimiter
type RateLimitRule = coresms.RateLimitRule

const (
	KeySMS = coresms.KeySMS
	Name   = string(KeySMS)
)

func FromDefault() (*Manager, error) {
	return coresms.ManagerDefault()
}

func From(app *runtime.App) *Manager {
	return coresms.From(app)
}
