//go:build sdkit_sms_twilio

package twilio

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/pkg/request"
	"github.com/huwenlong92/sdkit/pkg/sms"
)

const defaultEndpoint = "https://api.twilio.com/2010-04-01"

func init() {
	Register()
}

func Register() {
	sms.RegisterDriver("twilio", New)
}

type Provider struct {
	name   string
	config sms.ProviderConfig
	client *request.Client
}

func New(name string, cfg sms.ProviderConfig) (sms.Provider, error) {
	if cfg.Account == "" {
		return nil, errors.New("sms twilio: account sid is required")
	}
	if cfg.Password == "" {
		return nil, errors.New("sms twilio: auth token is required")
	}
	if cfg.Sender == "" && cfg.MessagingServiceSID == "" {
		return nil, errors.New("sms twilio: sender or messaging service sid is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	client, err := request.NewClient(request.WithTimeout(cfg.Timeout))
	if err != nil {
		return nil, err
	}
	return &Provider{name: name, config: cfg, client: client}, nil
}

func (p *Provider) Send(ctx context.Context, req sms.ProviderRequest) (*sms.ProviderResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	content := strings.TrimSpace(req.Payload.Content)
	if content == "" {
		return nil, errors.New("sms twilio: content is required")
	}
	if len(req.To) == 0 {
		return nil, errors.New("sms twilio: recipient is required")
	}

	responses := make([]twilioMessageResponse, 0, len(req.To))
	for _, to := range req.To {
		to = strings.TrimSpace(to)
		if to == "" {
			return nil, errors.New("sms twilio: recipient is required")
		}
		response, err := p.sendOne(ctx, to, content)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}

	result := &sms.ProviderResult{
		Provider: p.name,
		Success:  len(responses) > 0,
		Raw:      responses,
	}
	if len(responses) > 0 {
		result.Code = responses[0].Status
		result.Message = responses[0].Status
		result.MessageNo = responses[0].SID
	}
	return result, nil
}

func (p *Provider) sendOne(ctx context.Context, to string, content string) (twilioMessageResponse, error) {
	values := url.Values{
		"To":   {to},
		"Body": {content},
	}
	if p.config.MessagingServiceSID != "" {
		values.Set("MessagingServiceSid", p.config.MessagingServiceSID)
	} else {
		values.Set("From", p.config.Sender)
	}
	var response twilioMessageResponse
	_, err := p.client.Post(ctx, p.messagesURL(),
		request.WithBasicAuth(p.config.Account, p.config.Password),
		request.WithForm(values),
		request.WithDecodeJSON(&response),
	)
	if err != nil {
		return twilioMessageResponse{}, err
	}
	return response, nil
}

func (p *Provider) messagesURL() string {
	endpoint := strings.TrimRight(p.config.Endpoint, "/")
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return endpoint + "/Accounts/" + url.PathEscape(p.config.Account) + "/Messages.json"
}

func (p *Provider) Close() error {
	return nil
}

type twilioMessageResponse struct {
	SID          string `json:"sid"`
	Status       string `json:"status"`
	ErrorCode    any    `json:"error_code"`
	ErrorMessage any    `json:"error_message"`
	To           string `json:"to"`
	From         string `json:"from"`
	Body         string `json:"body"`
}
