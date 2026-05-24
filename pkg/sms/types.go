package sms

import (
	"context"
	"strings"
	"time"
)

type ProviderConfig struct {
	Driver          string        `mapstructure:"driver" yaml:"driver"`
	Account         string        `mapstructure:"account" yaml:"account"`
	Password        string        `mapstructure:"password" yaml:"password"`
	AppKey          string        `mapstructure:"app_key" yaml:"app_key"`
	AppSecret       string        `mapstructure:"app_secret" yaml:"app_secret"`
	AccessKeyID     string        `mapstructure:"access_key_id" yaml:"access_key_id"`
	AccessKeySecret string        `mapstructure:"access_key_secret" yaml:"access_key_secret"`
	RegionID        string        `mapstructure:"region_id" yaml:"region_id"`
	SignID          string        `mapstructure:"sign_id" yaml:"sign_id"`
	SignName        string        `mapstructure:"sign_name" yaml:"sign_name"`
	Endpoint        string        `mapstructure:"endpoint" yaml:"endpoint"`
	Timeout         time.Duration `mapstructure:"timeout" yaml:"timeout"`

	Name string `mapstructure:"-" yaml:"-"`
}

func (c ProviderConfig) Clone() ProviderConfig {
	return c
}

func (c ProviderConfig) AccessKey() string {
	if c.AccessKeyID != "" {
		return c.AccessKeyID
	}
	return c.AppKey
}

func (c ProviderConfig) SecretKey() string {
	if c.AccessKeySecret != "" {
		return c.AccessKeySecret
	}
	return c.AppSecret
}

func (c ProviderConfig) Region() string {
	if c.RegionID != "" {
		return c.RegionID
	}
	return "cn-hangzhou"
}

type Param struct {
	Key   string
	Value any
}

type Payload struct {
	Content  string
	Template string
	Data     []Param
}

func (p Payload) DataMap() map[string]any {
	if len(p.Data) == 0 {
		return nil
	}
	values := make(map[string]any, len(p.Data))
	for _, item := range p.Data {
		values[item.Key] = item.Value
	}
	return values
}

func (p Payload) DataValues() []any {
	if len(p.Data) == 0 {
		return nil
	}
	values := make([]any, 0, len(p.Data))
	for _, item := range p.Data {
		values = append(values, item.Value)
	}
	return values
}

type Message interface {
	Resolve(ctx context.Context, provider ProviderConfig) (Payload, error)
}

type ProviderMessage interface {
	Message
	Providers(ctx context.Context) []string
}

type TemplateMessage struct {
	Content       string
	Template      string
	Data          []Param
	ProviderNames []string
}

func (m TemplateMessage) Resolve(context.Context, ProviderConfig) (Payload, error) {
	return Payload{
		Content:  m.Content,
		Template: m.Template,
		Data:     append([]Param(nil), m.Data...),
	}, nil
}

func (m TemplateMessage) Providers(context.Context) []string {
	return cleanProviderNames(m.ProviderNames)
}

type Provider interface {
	Send(ctx context.Context, req ProviderRequest) (*ProviderResult, error)
	Close() error
}

type ProviderRequest struct {
	To       []string
	Message  Message
	Payload  Payload
	Provider ProviderConfig
}

type ProviderResult struct {
	Provider  string
	Success   bool
	Code      string
	Message   string
	MessageNo string
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
	To        []string
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

func cleanProviderNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	cleaned := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		cleaned = append(cleaned, name)
	}
	return cleaned
}
