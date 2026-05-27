package driver

import (
	"errors"
	"fmt"
	"strings"
)

var registerFuncs []registeredDriver

var ErrNoDriver = errors.New("queue: no driver compiled; build with sdkit_queue_asynq or sdkit_queue_nats")

type registeredDriver struct {
	name string
	fn   func() error
}

func Register() error {
	if len(registerFuncs) == 0 {
		return ErrNoDriver
	}
	if len(registerFuncs) > 1 {
		names := make([]string, 0, len(registerFuncs))
		for _, driver := range registerFuncs {
			names = append(names, driver.name)
		}
		return fmt.Errorf("queue: multiple drivers compiled (%s); enable only one queue build tag", strings.Join(names, ","))
	}
	return registerFuncs[0].fn()
}

func register(name string, fn func() error) {
	if name == "" || fn == nil {
		return
	}
	registerFuncs = append(registerFuncs, registeredDriver{name: name, fn: fn})
}
