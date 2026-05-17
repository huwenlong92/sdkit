package redis

import (
	"github.com/huwenlong92/sdkit/core/runtime"
)

const KeyRedis runtime.Key = "redis"

func From(app *runtime.App) *RuntimeClient {
	if app != nil {
		if value, ok := app.Container().Get(KeyRedis); ok {
			if client, ok := value.(*RuntimeClient); ok {
				return client
			}
		}
	}
	mu.Lock()
	defer mu.Unlock()
	return Default
}

func Bind(app *runtime.App, client *RuntimeClient) error {
	if client == nil {
		replaceDefault(nil)
		if app == nil {
			return nil
		}
		return runtime.ErrContainerValueNil
	}
	replaceDefault(client)
	if app == nil {
		return nil
	}
	return app.Container().Bind(KeyRedis, client)
}

func replaceDefault(client *RuntimeClient) {
	mu.Lock()
	defer mu.Unlock()

	if Default != nil && Default != client {
		_ = Default.Close()
	}
	Default = client
	if client == nil {
		RDB = nil
		return
	}
	RDB = client.Rdb
}
