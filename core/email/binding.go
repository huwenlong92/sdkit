package email

import "github.com/huwenlong92/sdkit/core/runtime"

const KeyEmail runtime.Key = "email"

func From(app *runtime.App) *Manager {
	if app != nil {
		if value, ok := app.Container().Get(KeyEmail); ok {
			if manager, ok := value.(*Manager); ok {
				return manager
			}
		}
	}
	manager, _ := ManagerDefault()
	return manager
}

func Bind(app *runtime.App, manager *Manager) error {
	if manager == nil {
		SetDefault(nil)
		if app == nil {
			return nil
		}
		return runtime.ErrContainerValueNil
	}
	SetDefault(manager)
	if app == nil {
		return nil
	}
	return app.Container().Bind(KeyEmail, manager)
}
