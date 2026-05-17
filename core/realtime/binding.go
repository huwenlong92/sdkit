package realtime

import "github.com/huwenlong92/sdkit/core/runtime"

const KeyRealtime runtime.Key = "realtime"

func From(app *runtime.App) Publisher {
	if app != nil {
		if value, ok := app.Container().Get(KeyRealtime); ok {
			if publisher, ok := value.(Publisher); ok {
				return publisher
			}
		}
	}
	return DefaultPublisher()
}

func Bind(app *runtime.App, publisher Publisher) error {
	if publisher == nil {
		SetDefaultPublisher(nil)
		if app == nil {
			return nil
		}
		return runtime.ErrContainerValueNil
	}
	SetDefaultPublisher(publisher)
	if app == nil {
		return nil
	}
	return app.Container().Bind(KeyRealtime, publisher)
}
