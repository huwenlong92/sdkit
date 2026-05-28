package email

import (
	"bytes"
	"context"
	"errors"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
	"time"
)

var (
	ErrMessageRequired          = errors.New("email: message is required")
	ErrTemplateRendererRequired = errors.New("email: template renderer is required")
	ErrTemplateFSRequired       = errors.New("email: template fs is required")
	ErrTemplateNotFound         = errors.New("email: template not found")
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

type Payload struct {
	To      []string
	Cc      []string
	Bcc     []string
	Subject string
	Text    string
	HTML    string
	Headers map[string]string
}

type Message interface {
	Resolve(ctx context.Context, renderer TemplateRenderer) (Payload, error)
}

type DirectMessage struct {
	To      []string
	Cc      []string
	Bcc     []string
	Subject string
	Text    string
	HTML    string
	Headers map[string]string
}

func (m DirectMessage) Resolve(context.Context, TemplateRenderer) (Payload, error) {
	return Payload{
		To:      append([]string(nil), m.To...),
		Cc:      append([]string(nil), m.Cc...),
		Bcc:     append([]string(nil), m.Bcc...),
		Subject: m.Subject,
		Text:    m.Text,
		HTML:    m.HTML,
		Headers: cloneHeaders(m.Headers),
	}, nil
}

type TemplateMessage struct {
	To       []string
	Cc       []string
	Bcc      []string
	Template string
	Data     map[string]any
	Headers  map[string]string
}

func (m TemplateMessage) Resolve(ctx context.Context, renderer TemplateRenderer) (Payload, error) {
	if renderer == nil {
		return Payload{}, ErrTemplateRendererRequired
	}
	tpl, err := renderer.Render(ctx, m.Template, m.Data)
	if err != nil {
		return Payload{}, err
	}
	return Payload{
		To:      append([]string(nil), m.To...),
		Cc:      append([]string(nil), m.Cc...),
		Bcc:     append([]string(nil), m.Bcc...),
		Subject: tpl.Subject,
		Text:    tpl.Text,
		HTML:    tpl.HTML,
		Headers: cloneHeaders(m.Headers),
	}, nil
}

type Template struct {
	Subject     string `mapstructure:"subject" yaml:"subject"`
	SubjectFile string `mapstructure:"subject_file" yaml:"subject_file"`
	Text        string `mapstructure:"text" yaml:"text"`
	TextFile    string `mapstructure:"text_file" yaml:"text_file"`
	HTML        string `mapstructure:"html" yaml:"html"`
	HTMLFile    string `mapstructure:"html_file" yaml:"html_file"`
}

type TemplateRenderer interface {
	Render(ctx context.Context, name string, data map[string]any) (Template, error)
}

type TemplateRendererFunc func(ctx context.Context, name string, data map[string]any) (Template, error)

func (fn TemplateRendererFunc) Render(ctx context.Context, name string, data map[string]any) (Template, error) {
	if fn == nil {
		return Template{}, ErrTemplateRendererRequired
	}
	return fn(ctx, name, data)
}

type TemplateMap map[string]Template

func (m TemplateMap) Render(ctx context.Context, name string, data map[string]any) (Template, error) {
	if err := ctx.Err(); err != nil {
		return Template{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Template{}, ErrTemplateNotFound
	}
	tpl, ok := m[name]
	if !ok {
		return Template{}, ErrTemplateNotFound
	}
	rendered, err := renderTemplate(tpl, data)
	if err != nil {
		return Template{}, err
	}
	return rendered, nil
}

type Provider interface {
	Send(ctx context.Context, payload Payload) (*ProviderResult, error)
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
	Result   *ProviderResult
	Error    error
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

func renderTemplate(tpl Template, data map[string]any) (Template, error) {
	subject, err := renderTextTemplate("subject", tpl.Subject, data)
	if err != nil {
		return Template{}, err
	}
	text, err := renderTextTemplate("text", tpl.Text, data)
	if err != nil {
		return Template{}, err
	}
	html, err := renderHTMLTemplate("html", tpl.HTML, data)
	if err != nil {
		return Template{}, err
	}
	return Template{Subject: subject, Text: text, HTML: html}, nil
}

func renderTextTemplate(name string, body string, data map[string]any) (string, error) {
	if body == "" {
		return "", nil
	}
	tpl, err := texttemplate.New(name).Parse(body)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderHTMLTemplate(name string, body string, data map[string]any) (string, error) {
	if body == "" {
		return "", nil
	}
	tpl, err := htmltemplate.New(name).Parse(body)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	clone := make(map[string]string, len(headers))
	for key, value := range headers {
		clone[key] = value
	}
	return clone
}
