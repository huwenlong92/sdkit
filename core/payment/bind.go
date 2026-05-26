package payment

import "github.com/huwenlong92/sdkit/core/runtime"

const KeyPayment runtime.Key = "payment"

func From(app *runtime.App) *Service {
	if app != nil {
		if value, ok := app.Container().Get(KeyPayment); ok {
			if service, ok := value.(*Service); ok {
				return service
			}
		}
	}
	service, _ := Default()
	return service
}

func Bind(app *runtime.App, service *Service) error {
	if service == nil {
		SetDefault(nil)
		if app == nil {
			return nil
		}
		return runtime.ErrContainerValueNil
	}
	SetDefault(service)
	if app == nil {
		return nil
	}
	return app.Container().Bind(KeyPayment, service)
}
