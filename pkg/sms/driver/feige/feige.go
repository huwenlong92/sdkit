//go:build sdkit_sms_feige

package feige

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/pkg/request"
	"github.com/huwenlong92/sdkit/pkg/sms"
)

const defaultEndpoint = "https://api.4321.sh/sms/template"

func init() {
	Register()
}

func Register() {
	sms.RegisterDriver("feige", New)
}

type Provider struct {
	name   string
	config sms.ProviderConfig
	client *request.Client
}

func New(name string, cfg sms.ProviderConfig) (sms.Provider, error) {
	if cfg.Account == "" {
		return nil, errors.New("sms feige: account is required")
	}
	if cfg.Password == "" {
		return nil, errors.New("sms feige: password is required")
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
	if len(req.To) == 0 || strings.TrimSpace(req.To[0]) == "" {
		return nil, errors.New("sms feige: phone is required")
	}
	endpoint := p.config.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	values := req.Payload.DataValues()
	content := make([]string, 0, len(values))
	for _, value := range values {
		content = append(content, fmt.Sprint(value))
	}
	var response struct {
		MsgNo   string  `json:"msg_no"`
		Fee     float64 `json:"fee"`
		Code    int64   `json:"code"`
		Message string  `json:"msg"`
		Count   int32   `json:"count"`
	}
	resp, err := p.client.Post(ctx, endpoint,
		request.WithJSON(map[string]any{
			"apikey":      p.config.Account,
			"secret":      p.config.Password,
			"sign_id":     p.config.SignID,
			"template_id": req.Payload.Template,
			"mobile":      req.To[0],
			"content":     strings.Join(content, "||"),
		}),
		request.WithDecodeJSON(&response),
	)
	if err != nil {
		return nil, err
	}
	result := &sms.ProviderResult{
		Provider:  p.name,
		Success:   response.Code == 0,
		Code:      fmt.Sprint(response.Code),
		Message:   response.Message,
		MessageNo: response.MsgNo,
		Raw:       response,
	}
	if resp != nil && result.Raw == nil {
		var raw map[string]any
		_ = json.Unmarshal(resp.Bytes(), &raw)
		result.Raw = raw
	}
	return result, nil
}

func (p *Provider) Close() error {
	return nil
}
