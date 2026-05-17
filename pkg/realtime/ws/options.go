package ws

import (
	"context"
	"net/http"
	"time"

	"github.com/huwenlong92/sdkit/core/realtime"
	"github.com/huwenlong92/sdkit/pkg/realtime/transport"
)

type Registry interface {
	Add(c *realtime.Client) error
	Remove(clientID string) error
}

type MessageHandler func(context.Context, *realtime.Client, []byte) error

type ClientFactory func(*http.Request) *realtime.Client

type Options struct {
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	PingInterval     time.Duration
	ClientBufferSize int
	MaxMessageSize   int64
	Logger           transport.Logger
	Lifecycle        transport.Lifecycle
	OnMessage        MessageHandler
	NewClient        ClientFactory
}

func (o Options) normalize() Options {
	if o.ClientBufferSize <= 0 {
		o.ClientBufferSize = 64
	}
	if o.MaxMessageSize <= 0 {
		o.MaxMessageSize = 1 << 20
	}
	if o.PingInterval < 0 {
		o.PingInterval = 0
	}
	if o.Logger == nil {
		o.Logger = transport.NopLogger{}
	}
	return o
}
