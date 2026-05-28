//go:build sdkit_sms_huawei

package huawei

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/pkg/request"
	"github.com/huwenlong92/sdkit/pkg/sms"
)

const sendPath = "/sms/batchSendSms/v1"

func init() {
	Register()
}

func Register() {
	sms.RegisterDriver("huawei", New)
}

type Provider struct {
	name     string
	config   sms.ProviderConfig
	endpoint string
	client   *request.Client
}

func New(name string, cfg sms.ProviderConfig) (sms.Provider, error) {
	if cfg.AccessKey() == "" {
		return nil, errors.New("sms huawei: app key is required")
	}
	if cfg.SecretKey() == "" {
		return nil, errors.New("sms huawei: app secret is required")
	}
	if cfg.Sender == "" {
		return nil, errors.New("sms huawei: sender is required")
	}
	if cfg.Endpoint == "" {
		return nil, errors.New("sms huawei: endpoint is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if !strings.HasSuffix(endpoint, sendPath) {
		endpoint += sendPath
	}
	client, err := request.NewClient(request.WithTimeout(cfg.Timeout))
	if err != nil {
		return nil, err
	}
	return &Provider{name: name, config: cfg, endpoint: endpoint, client: client}, nil
}

func (p *Provider) Send(ctx context.Context, req sms.ProviderRequest) (*sms.ProviderResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(req.To) == 0 {
		return nil, errors.New("sms huawei: recipient is required")
	}
	if req.Payload.Template == "" {
		return nil, errors.New("sms huawei: template id is required")
	}

	values := url.Values{
		"from":       {p.config.Sender},
		"to":         {strings.Join(cleanRecipients(req.To), ",")},
		"templateId": {req.Payload.Template},
	}
	if p.config.SignName != "" {
		values.Set("signature", p.config.SignName)
	}
	params := paramValues(req.Payload.Data)
	if len(params) > 0 {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		values.Set("templateParas", string(b))
	}

	var response sendSMSResponse
	_, err := p.client.Post(ctx, p.endpoint,
		request.WithHeader("Authorization", `WSSE realm="SDP",profile="UsernameToken",type="Appkey"`),
		request.WithHeader("X-WSSE", p.wsseHeader(time.Now().UTC())),
		request.WithForm(values),
		request.WithDecodeJSON(&response),
	)
	if err != nil {
		return nil, err
	}
	return p.providerResult(response), nil
}

func (p *Provider) wsseHeader(now time.Time) string {
	nonce := randomNonce()
	created := now.Format("2006-01-02T15:04:05Z")
	digest := passwordDigest(nonce, created, p.config.SecretKey())
	return `UsernameToken Username="` + p.config.AccessKey() + `", PasswordDigest="` + digest + `", Nonce="` + nonce + `", Created="` + created + `"`
}

func (p *Provider) providerResult(response sendSMSResponse) *sms.ProviderResult {
	result := &sms.ProviderResult{
		Provider:  p.name,
		Success:   response.Code == "000000",
		Code:      response.Code,
		Message:   response.Description,
		MessageNo: firstMessageNo(response.Result),
		Raw:       response,
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

func firstMessageNo(values []smsID) string {
	if len(values) == 0 {
		return ""
	}
	return values[0].SMSMsgID
}

func passwordDigest(nonce string, created string, password string) string {
	sum := sha256.Sum256([]byte(nonce + created + password))
	hexValue := hex.EncodeToString(sum[:])
	return base64.StdEncoding.EncodeToString([]byte(hexValue))
}

func randomNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

type sendSMSResponse struct {
	Code        string  `json:"code"`
	Description string  `json:"description"`
	Result      []smsID `json:"result"`
}

type smsID struct {
	SMSMsgID   string `json:"smsMsgId"`
	From       string `json:"from"`
	OriginTo   string `json:"originTo"`
	Status     string `json:"status"`
	CreateTime string `json:"createTime"`
	CountryID  string `json:"countryId"`
	Total      int64  `json:"total"`
}
