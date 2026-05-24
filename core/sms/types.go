package sms

import pkgsms "github.com/huwenlong92/sdkit/pkg/sms"

type ProviderConfig = pkgsms.ProviderConfig
type Param = pkgsms.Param
type Payload = pkgsms.Payload
type Message = pkgsms.Message
type ProviderMessage = pkgsms.ProviderMessage
type TemplateMessage = pkgsms.TemplateMessage
type Provider = pkgsms.Provider
type ProviderRequest = pkgsms.ProviderRequest
type ProviderResult = pkgsms.ProviderResult
type AttemptResult = pkgsms.AttemptResult
type SendResult = pkgsms.SendResult
type Request = pkgsms.Request
type Sender = pkgsms.Sender
type SenderFunc = pkgsms.SenderFunc
type Middleware = pkgsms.Middleware

func RegisterDriver(driver string, factory pkgsms.DriverFactory) {
	pkgsms.RegisterDriver(driver, factory)
}
