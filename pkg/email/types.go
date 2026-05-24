package email

import (
	"context"
	"time"
)

type ProviderConfig struct {
	Driver      string        `mapstructure:"driver" yaml:"driver"`
	Host        string        `mapstructure:"host" yaml:"host"`
	Port        int           `mapstructure:"port" yaml:"port"`
	Username    string        `mapstructure:"username" yaml:"username"`
	Password    string        `mapstructure:"password" yaml:"password"`
	FromAddress string        `mapstructure:"from_address" yaml:"from_address"`
	FromName    string        `mapstructure:"from_name" yaml:"from_name"`
	ReplyTo     string        `mapstructure:"reply_to" yaml:"reply_to"`
	Encryption  string        `mapstructure:"encryption" yaml:"encryption"`
	Auth        string        `mapstructure:"auth" yaml:"auth"`
	Timeout     time.Duration `mapstructure:"timeout" yaml:"timeout"`
}

func (c ProviderConfig) Clone() ProviderConfig {
	return c
}

type Message struct {
	To      []string
	Cc      []string
	Bcc     []string
	Subject string
	Text    string
	HTML    string
	Headers map[string]string
}

type Provider interface {
	Send(ctx context.Context, msg Message) (*ProviderResult, error)
	Close() error
}

type ProviderResult struct {
	MessageID string
	Raw       any
}

type AttemptResult struct {
	Provider string
	Success  bool
	Result   *ProviderResult
	Error    error
}

type SendResult struct {
	Provider string
	Attempts []AttemptResult
}

type Request struct {
	Message   Message
	Providers []string
}

type Sender interface {
	Send(ctx context.Context, req Request) (*SendResult, error)
}

type SenderFunc func(ctx context.Context, req Request) (*SendResult, error)

func (fn SenderFunc) Send(ctx context.Context, req Request) (*SendResult, error) {
	return fn(ctx, req)
}

type Middleware func(Sender) Sender
