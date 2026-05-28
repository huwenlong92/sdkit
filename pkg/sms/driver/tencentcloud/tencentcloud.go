//go:build sdkit_sms_tencentcloud

package tencentcloud

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/pkg/request"
	"github.com/huwenlong92/sdkit/pkg/sms"
)

const (
	defaultEndpoint = "https://sms.tencentcloudapi.com"
	service         = "sms"
	action          = "SendSms"
	version         = "2019-07-11"
	algorithm       = "TC3-HMAC-SHA256"
	contentType     = "application/json"
)

func init() {
	Register()
}

func Register() {
	sms.RegisterDriver("tencentcloud", New)
}

type Provider struct {
	name     string
	config   sms.ProviderConfig
	endpoint string
	host     string
	client   *request.Client
}

func New(name string, cfg sms.ProviderConfig) (sms.Provider, error) {
	if cfg.AccessKey() == "" {
		return nil, errors.New("sms tencentcloud: secret id is required")
	}
	if cfg.SecretKey() == "" {
		return nil, errors.New("sms tencentcloud: secret key is required")
	}
	if cfg.SmsSdkAppID == "" {
		return nil, errors.New("sms tencentcloud: sms sdk app id is required")
	}
	if cfg.SignName == "" {
		return nil, errors.New("sms tencentcloud: sign name is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	if parsed.Host == "" {
		return nil, errors.New("sms tencentcloud: endpoint host is required")
	}
	client, err := request.NewClient(request.WithTimeout(cfg.Timeout))
	if err != nil {
		return nil, err
	}
	return &Provider{name: name, config: cfg, endpoint: endpoint, host: parsed.Host, client: client}, nil
}

func (p *Provider) Send(ctx context.Context, req sms.ProviderRequest) (*sms.ProviderResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(req.To) == 0 {
		return nil, errors.New("sms tencentcloud: recipient is required")
	}
	if req.Payload.Template == "" {
		return nil, errors.New("sms tencentcloud: template id is required")
	}

	body := sendSMSRequest{
		PhoneNumberSet:   cleanRecipients(req.To),
		SmsSdkAppID:      p.config.SmsSdkAppID,
		Sign:             p.config.SignName,
		TemplateID:       req.Payload.Template,
		TemplateParamSet: paramValues(req.Payload.Data),
	}
	if p.config.Sender != "" {
		body.SenderID = p.config.Sender
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	timestamp := time.Now().Unix()
	headers := p.headers(timestamp, string(payload))

	var response sendSMSResponse
	_, err = p.client.Post(ctx, p.endpoint,
		request.WithHeaders(headers),
		request.WithBytes(contentType, payload),
		request.WithDecodeJSON(&response),
	)
	if err != nil {
		return nil, err
	}
	return p.providerResult(response), nil
}

func (p *Provider) headers(timestamp int64, payload string) http.Header {
	headers := make(http.Header)
	headers.Set("Authorization", p.authorization(timestamp, payload))
	headers.Set("X-TC-Action", action)
	headers.Set("X-TC-Timestamp", fmt.Sprint(timestamp))
	headers.Set("X-TC-Version", version)
	if p.config.RegionID != "" {
		headers.Set("X-TC-Region", p.config.RegionID)
	}
	return headers
}

func (p *Provider) authorization(timestamp int64, payload string) string {
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	canonicalHeaders := "content-type:" + contentType + "\n" + "host:" + p.host + "\n"
	signedHeaders := "content-type;host"
	canonicalRequest := strings.Join([]string{
		http.MethodPost,
		"/",
		"",
		canonicalHeaders,
		signedHeaders,
		sha256Hex(payload),
	}, "\n")
	credentialScope := date + "/" + service + "/tc3_request"
	stringToSign := strings.Join([]string{
		algorithm,
		fmt.Sprint(timestamp),
		credentialScope,
		sha256Hex(canonicalRequest),
	}, "\n")
	secretDate := hmacSHA256([]byte("TC3"+p.config.SecretKey()), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))
	return algorithm + " Credential=" + p.config.AccessKey() + "/" + credentialScope + ", SignedHeaders=" + signedHeaders + ", Signature=" + signature
}

func (p *Provider) providerResult(response sendSMSResponse) *sms.ProviderResult {
	result := &sms.ProviderResult{
		Provider: p.name,
		Raw:      response,
	}
	if response.Response.Error != nil {
		result.Code = response.Response.Error.Code
		result.Message = response.Response.Error.Message
		return result
	}
	statuses := response.Response.SendStatusSet
	result.Success = len(statuses) > 0
	if len(statuses) > 0 {
		result.Code = statuses[0].Code
		result.Message = statuses[0].Message
		result.MessageNo = statuses[0].SerialNo
	}
	for _, status := range statuses {
		if !strings.EqualFold(status.Code, "Ok") {
			result.Success = false
			break
		}
	}
	return result
}

func (p *Provider) Close() error {
	return nil
}

func cleanRecipients(values []string) []string {
	recipients := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			recipients = append(recipients, value)
		}
	}
	return recipients
}

func paramValues(params []sms.Param) []string {
	if len(params) == 0 {
		return nil
	}
	values := make([]string, 0, len(params))
	for _, param := range params {
		values = append(values, fmt.Sprint(param.Value))
	}
	return values
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

type sendSMSRequest struct {
	PhoneNumberSet   []string `json:"PhoneNumberSet"`
	SmsSdkAppID      string   `json:"SmsSdkAppid"`
	Sign             string   `json:"Sign"`
	TemplateID       string   `json:"TemplateID"`
	TemplateParamSet []string `json:"TemplateParamSet,omitempty"`
	SenderID         string   `json:"SenderId,omitempty"`
}

type sendSMSResponse struct {
	Response struct {
		SendStatusSet []sendStatus `json:"SendStatusSet"`
		RequestID     string       `json:"RequestId"`
		Error         *apiError    `json:"Error"`
	} `json:"Response"`
}

type sendStatus struct {
	SerialNo       string `json:"SerialNo"`
	PhoneNumber    string `json:"PhoneNumber"`
	Fee            int64  `json:"Fee"`
	SessionContext string `json:"SessionContext"`
	Code           string `json:"Code"`
	Message        string `json:"Message"`
	IsoCode        string `json:"IsoCode"`
}

type apiError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}
