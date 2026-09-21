//go:build sdkit_sms_aliyun_global

package aliyunglobal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/huwenlong92/sdkit/pkg/sms"
)

const (
	driverName      = "aliyun_global"
	defaultRegion   = "ap-southeast-1"
	defaultEndpoint = "dysmsapi.ap-southeast-1.aliyuncs.com"
)

func init() {
	Register()
}

// Register registers the Alibaba Cloud international SMS driver.
func Register() {
	sms.RegisterDriver(driverName, New)
}

// Provider sends international SMS messages through SendMessageToGlobe.
type Provider struct {
	name     string
	config   sms.ProviderConfig
	client   *sdk.Client
	region   string
	scheme   string
	endpoint string
}

type providerResponse struct {
	Message             string `json:"Message"`
	RequestID           string `json:"RequestId"`
	Code                string `json:"Code"`
	BizID               string `json:"BizId"`
	ResponseCode        string `json:"ResponseCode"`
	ResponseDescription string `json:"ResponseDescription"`
	MessageID           string `json:"MessageId"`
	From                string `json:"From"`
	To                  string `json:"To"`
	Segments            string `json:"Segments"`
	NumberDetail        any    `json:"NumberDetail"`
}

// New creates an Alibaba Cloud international SMS provider.
func New(name string, config sms.ProviderConfig) (sms.Provider, error) {
	if strings.TrimSpace(config.AccessKey()) == "" {
		return nil, errors.New("sms aliyun global: access key is required")
	}
	if strings.TrimSpace(config.SecretKey()) == "" {
		return nil, errors.New("sms aliyun global: access secret is required")
	}
	region := strings.TrimSpace(config.RegionID)
	if region == "" {
		region = defaultRegion
	}
	scheme, endpoint, err := normalizeEndpoint(config.Endpoint)
	if err != nil {
		return nil, err
	}
	client, err := sdk.NewClientWithAccessKey(region, config.AccessKey(), config.SecretKey())
	if err != nil {
		return nil, err
	}
	return &Provider{name: name, config: config, client: client, region: region, scheme: scheme, endpoint: endpoint}, nil
}

// Send sends one rendered message to one international phone number.
func (provider *Provider) Send(ctx context.Context, request sms.ProviderRequest) (*sms.ProviderResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(request.To) != 1 || strings.TrimSpace(request.To[0]) == "" {
		return nil, errors.New("sms aliyun global: exactly one recipient is required")
	}
	content := strings.TrimSpace(request.Payload.Content)
	if content == "" {
		return nil, errors.New("sms aliyun global: content is required")
	}
	commonRequest := buildRequest(provider.region, provider.scheme, provider.endpoint, request.To[0], content, provider.config.Sender)
	response, err := provider.client.ProcessCommonRequest(commonRequest)
	if err != nil {
		return nil, err
	}
	return parseResponse(provider.name, response.GetHttpContentBytes())
}

// Close releases provider resources.
func (provider *Provider) Close() error { return nil }

func buildRequest(region string, scheme string, endpoint string, recipient string, content string, sender string) *requests.CommonRequest {
	request := requests.NewCommonRequest()
	request.Method = requests.POST
	request.Scheme = scheme
	request.Domain = endpoint
	request.Version = "2018-05-01"
	request.ApiName = "SendMessageToGlobe"
	request.QueryParams["RegionId"] = region
	request.QueryParams["To"] = recipient
	request.QueryParams["Message"] = content
	if sender = strings.TrimSpace(sender); sender != "" {
		request.QueryParams["From"] = sender
	}
	return request
}

func parseResponse(providerName string, data []byte) (*sms.ProviderResult, error) {
	var response providerResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("sms aliyun global: decode response: %w", err)
	}
	code := strings.TrimSpace(response.ResponseCode)
	if code == "" {
		code = strings.TrimSpace(response.Code)
	}
	message := strings.TrimSpace(response.ResponseDescription)
	if message == "" {
		message = strings.TrimSpace(response.Message)
	}
	messageNo := strings.TrimSpace(response.MessageID)
	if messageNo == "" {
		messageNo = strings.TrimSpace(response.BizID)
	}
	return &sms.ProviderResult{
		Provider:  providerName,
		Success:   code == "OK",
		Code:      code,
		Message:   message,
		MessageNo: messageNo,
		Raw:       response,
	}, nil
}

func normalizeEndpoint(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "https", defaultEndpoint, nil
	}
	scheme := "https"
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" || parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", "", errors.New("sms aliyun global: endpoint must not contain a path")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return "", "", errors.New("sms aliyun global: endpoint scheme must be http or https")
		}
		scheme = parsed.Scheme
		value = parsed.Host
	}
	if strings.ContainsAny(value, "/?#") {
		return "", "", errors.New("sms aliyun global: endpoint must not contain a path")
	}
	return scheme, value, nil
}

var _ sms.Provider = (*Provider)(nil)
