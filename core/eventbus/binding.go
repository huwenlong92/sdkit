package eventbus

import "github.com/huwenlong92/sdkit/core/runtime"

const KeyEventBus runtime.Key = "eventbus"

func From(app *runtime.App) Service {
	if app != nil {
		if value, ok := app.Container().Get(KeyEventBus); ok {
			if service, ok := value.(Service); ok {
				return service
			}
		}
	}
	return Default()
}

func Bind(app *runtime.App, bus Service) error {
	return BindWithDriver(app, bus, "")
}

func BindWithDriver(app *runtime.App, bus Service, driver string) error {
	if bus == nil {
		SetDefaultWithDriver(nil, "")
		if app == nil {
			return nil
		}
		return runtime.ErrContainerValueNil
	}
	defaultBus := bus
	if provider, ok := bus.(interface{ Bus() Bus }); ok {
		if inner := provider.Bus(); inner != nil {
			defaultBus = inner
		}
	}
	SetDefaultWithDriver(defaultBus, driver)
	if app == nil {
		return nil
	}
	return app.Container().Bind(KeyEventBus, bus)
}
