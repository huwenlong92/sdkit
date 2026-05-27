//go:build sdkit_sms_aliyun

package aliyun

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/huwenlong92/sdkit/pkg/sms"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/dysmsapi"
)

func init() {
	Register()
}

func Register() {
	sms.RegisterDriver("aliyun", New)
}

type Provider struct {
	name   string
	config sms.ProviderConfig
	client *dysmsapi.Client
}

func New(name string, cfg sms.ProviderConfig) (sms.Provider, error) {
	if cfg.AccessKey() == "" {
		return nil, errors.New("sms aliyun: access key is required")
	}
	if cfg.SecretKey() == "" {
		return nil, errors.New("sms aliyun: access secret is required")
	}
	if cfg.SignName == "" {
		return nil, errors.New("sms aliyun: sign name is required")
	}
	client, err := dysmsapi.NewClientWithAccessKey(cfg.Region(), cfg.AccessKey(), cfg.SecretKey())
	if err != nil {
		return nil, err
	}
	return &Provider{name: name, config: cfg, client: client}, nil
}

func (p *Provider) Send(ctx context.Context, req sms.ProviderRequest) (*sms.ProviderResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	params, err := json.Marshal(req.Payload.DataMap())
	if err != nil {
		return nil, err
	}
	request := dysmsapi.CreateSendSmsRequest()
	request.PhoneNumbers = strings.Join(req.To, ",")
	request.SignName = p.config.SignName
	request.TemplateCode = req.Payload.Template
	request.TemplateParam = string(params)
	response, err := p.client.SendSms(request)
	if err != nil {
		return nil, err
	}
	result := &sms.ProviderResult{
		Provider:  p.name,
		Success:   response.Code == "OK" && response.Message == "OK",
		Code:      response.Code,
		Message:   response.Message,
		MessageNo: response.BizId,
		Raw:       response,
	}
	return result, nil
}

func (p *Provider) Close() error {
	return nil
}
