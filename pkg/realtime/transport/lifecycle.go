package transport

import (
	"context"
	"sync"

	"github.com/huwenlong92/sdkit/core/realtime"
)

type Logger interface {
	Warn(msg string, fields ...any)
}

type NopLogger struct{}

func (NopLogger) Warn(string, ...any) {}

func NormalizeLogger(log Logger) Logger {
	if log == nil {
		return NopLogger{}
	}
	return log
}

type Lifecycle interface {
	OnConnect(ctx context.Context, client *realtime.Client) error
	OnDisconnect(ctx context.Context, client *realtime.Client) error
	OnActivity(ctx context.Context, client *realtime.Client) error
}

type LifecycleHooks struct {
	Connect    func(context.Context, *realtime.Client) error
	Disconnect func(context.Context, *realtime.Client) error
	Activity   func(context.Context, *realtime.Client) error
}

func (h LifecycleHooks) OnConnect(ctx context.Context, client *realtime.Client) error {
	if h.Connect == nil {
		return nil
	}
	return h.Connect(ctx, client)
}

func (h LifecycleHooks) OnDisconnect(ctx context.Context, client *realtime.Client) error {
	if h.Disconnect == nil {
		return nil
	}
	return h.Disconnect(ctx, client)
}

func (h LifecycleHooks) OnActivity(ctx context.Context, client *realtime.Client) error {
	if h.Activity == nil {
		return nil
	}
	return h.Activity(ctx, client)
}

type Closer struct {
	once   sync.Once
	cancel context.CancelFunc
	close  func() error
	err    error
}

func NewCloser(cancel context.CancelFunc, close func() error) *Closer {
	return &Closer{cancel: cancel, close: close}
}

func (c *Closer) Close() error {
	if c == nil {
		return nil
	}
	c.once.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		if c.close != nil {
			c.err = c.close()
		}
	})
	return c.err
}
