package runtime

import (
	"errors"
	"sync"
)

var (
	ErrContainerNil         = errors.New("runtime: container is nil")
	ErrContainerKeyRequired = errors.New("runtime: container key is required")
	ErrContainerValueNil    = errors.New("runtime: container value is nil")
)

type Container struct {
	values sync.Map
}

type Key string

func NewContainer() *Container {
	return &Container{}
}

func (c *Container) Bind(key Key, value any) error {
	if c == nil {
		return ErrContainerNil
	}
	if key == "" {
		return ErrContainerKeyRequired
	}
	if value == nil {
		return ErrContainerValueNil
	}
	c.values.Store(key, value)
	return nil
}

func (c *Container) Get(key Key) (any, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	return c.values.Load(key)
}

func (c *Container) MustGet(key Key) any {
	value, _ := c.Get(key)
	return value
}
