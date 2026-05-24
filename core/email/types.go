package email

import pkgemail "github.com/huwenlong92/sdkit/pkg/email"

type ProviderConfig = pkgemail.ProviderConfig
type Message = pkgemail.Message
type Provider = pkgemail.Provider
type ProviderResult = pkgemail.ProviderResult
type AttemptResult = pkgemail.AttemptResult
type SendResult = pkgemail.SendResult
type Request = pkgemail.Request
type Sender = pkgemail.Sender
type SenderFunc = pkgemail.SenderFunc
type Middleware = pkgemail.Middleware

func RegisterDriver(driver string, factory pkgemail.DriverFactory) {
	pkgemail.RegisterDriver(driver, factory)
}
