//go:build sdkit_payment_wechat

package apiv3

import (
	"context"
	"fmt"
	"strings"

	"github.com/huwenlong92/sdkit/core/payment"
	wechatcore "github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/app"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/h5"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/jsapi"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/services/refunddomestic"
)

func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.AppID) == "" {
		return nil, fmt.Errorf("%w: wechat app_id is required", payment.ErrInvalidRequest)
	}
	if strings.TrimSpace(cfg.MerchantID) == "" {
		return nil, fmt.Errorf("%w: wechat merchant_id is required", payment.ErrInvalidRequest)
	}
	if strings.TrimSpace(cfg.NotifyURL) == "" {
		return nil, fmt.Errorf("%w: wechat notify_url is required", payment.ErrInvalidRequest)
	}

	coreClient := cfg.CoreClient
	if coreClient == nil && servicesMissing(cfg) {
		var err error
		coreClient, err = newOfficialCoreClient(context.Background(), cfg)
		if err != nil {
			return nil, err
		}
	}

	client := &Client{
		appID:         strings.TrimSpace(cfg.AppID),
		merchantID:    strings.TrimSpace(cfg.MerchantID),
		notifyURL:     strings.TrimSpace(cfg.NotifyURL),
		appService:    cfg.AppService,
		jsapiService:  cfg.JSAPIService,
		h5Service:     cfg.H5Service,
		nativeService: cfg.NativeService,
		refundService: cfg.RefundService,
	}
	if coreClient != nil {
		if client.appService == nil {
			client.appService = &app.AppApiService{Client: coreClient}
		}
		if client.jsapiService == nil {
			client.jsapiService = &jsapi.JsapiApiService{Client: coreClient}
		}
		if client.h5Service == nil {
			client.h5Service = &h5.H5ApiService{Client: coreClient}
		}
		if client.nativeService == nil {
			client.nativeService = &native.NativeApiService{Client: coreClient}
		}
		if client.refundService == nil {
			client.refundService = &refunddomestic.RefundsApiService{Client: coreClient}
		}
	}
	return client, nil
}

func (c *Client) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	switch req.Channel {
	case payment.ChannelWechatApp:
		return c.createApp(ctx, req)
	case payment.ChannelWechatMiniProgram:
		return c.createJSAPI(ctx, req)
	case payment.ChannelWechatH5:
		return c.createH5(ctx, req)
	case payment.ChannelWechatNative:
		return c.createNative(ctx, req)
	default:
		return nil, fmt.Errorf("%w: wechat channel %s", payment.ErrUnsupportedChannel, req.Channel)
	}
}

func (c *Client) createApp(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if c.appService == nil {
		return nil, fmt.Errorf("%w: wechat app service is required", payment.ErrAdapterNotFound)
	}
	resp, _, err := c.appService.PrepayWithRequestPayment(ctx, c.appRequest(req))
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("%w: nil wechat app response", payment.ErrPaymentActionRequired)
	}
	return createResponse(req, payment.PaymentAction{
		Type: payment.ActionSDKParams,
		Params: map[string]any{
			"appid":     c.appID,
			"partnerid": stringValue(resp.PartnerId),
			"prepayid":  stringValue(resp.PrepayId),
			"package":   stringValue(resp.Package),
			"noncestr":  stringValue(resp.NonceStr),
			"timestamp": stringValue(resp.TimeStamp),
			"sign":      stringValue(resp.Sign),
		},
	}), nil
}

func (c *Client) createJSAPI(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if c.jsapiService == nil {
		return nil, fmt.Errorf("%w: wechat jsapi service is required", payment.ErrAdapterNotFound)
	}
	openID := stringExtra(req.Extra, ExtraOpenIDKey)
	if openID == "" {
		return nil, fmt.Errorf("%w: wechat mini program openid is required", payment.ErrInvalidRequest)
	}
	officialReq := c.jsapiRequest(req)
	officialReq.Payer = &jsapi.Payer{Openid: &openID}
	resp, _, err := c.jsapiService.PrepayWithRequestPayment(ctx, officialReq)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("%w: nil wechat jsapi response", payment.ErrPaymentActionRequired)
	}
	return createResponse(req, payment.PaymentAction{
		Type: payment.ActionSDKParams,
		Params: map[string]any{
			"appId":     stringValue(resp.Appid),
			"timeStamp": stringValue(resp.TimeStamp),
			"nonceStr":  stringValue(resp.NonceStr),
			"package":   stringValue(resp.Package),
			"signType":  stringValue(resp.SignType),
			"paySign":   stringValue(resp.PaySign),
		},
	}), nil
}

func (c *Client) createH5(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if c.h5Service == nil {
		return nil, fmt.Errorf("%w: wechat h5 service is required", payment.ErrAdapterNotFound)
	}
	clientIP := stringExtra(req.Extra, ExtraClientIPKey)
	if clientIP == "" {
		return nil, fmt.Errorf("%w: wechat h5 client_ip is required", payment.ErrInvalidRequest)
	}
	officialReq := c.h5Request(req)
	officialReq.SceneInfo = &h5.SceneInfo{
		PayerClientIp: &clientIP,
		H5Info:        &h5.H5Info{Type: stringPtr("Wap")},
	}
	resp, _, err := c.h5Service.Prepay(ctx, officialReq)
	if err != nil {
		return nil, err
	}
	if resp == nil || stringValue(resp.H5Url) == "" {
		return nil, fmt.Errorf("%w: wechat h5_url is required", payment.ErrPaymentActionRequired)
	}
	return createResponse(req, payment.PaymentAction{Type: payment.ActionRedirectURL, URL: stringValue(resp.H5Url)}), nil
}

func (c *Client) createNative(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if c.nativeService == nil {
		return nil, fmt.Errorf("%w: wechat native service is required", payment.ErrAdapterNotFound)
	}
	resp, _, err := c.nativeService.Prepay(ctx, c.nativeRequest(req))
	if err != nil {
		return nil, err
	}
	if resp == nil || stringValue(resp.CodeUrl) == "" {
		return nil, fmt.Errorf("%w: wechat code_url is required", payment.ErrPaymentActionRequired)
	}
	return createResponse(req, payment.PaymentAction{Type: payment.ActionQRCode, URL: stringValue(resp.CodeUrl)}), nil
}

func (c *Client) appRequest(req payment.CreatePaymentRequest) app.PrepayRequest {
	return app.PrepayRequest{
		Appid:       &c.appID,
		Mchid:       &c.merchantID,
		Description: stringPtr(description(req)),
		OutTradeNo:  stringPtr(outTradeNo(req)),
		TimeExpire:  req.ExpireAt,
		Attach:      optionalString(req.OrderID),
		NotifyUrl:   stringPtr(firstNonEmpty(req.NotifyURL, c.notifyURL)),
		Amount:      &app.Amount{Total: &req.Pricing.PayAmount.Amount, Currency: stringPtr(payment.NormalizeCurrency(req.Pricing.PayAmount.Currency))},
	}
}

func (c *Client) jsapiRequest(req payment.CreatePaymentRequest) jsapi.PrepayRequest {
	return jsapi.PrepayRequest{
		Appid:       &c.appID,
		Mchid:       &c.merchantID,
		Description: stringPtr(description(req)),
		OutTradeNo:  stringPtr(outTradeNo(req)),
		TimeExpire:  req.ExpireAt,
		Attach:      optionalString(req.OrderID),
		NotifyUrl:   stringPtr(firstNonEmpty(req.NotifyURL, c.notifyURL)),
		Amount:      &jsapi.Amount{Total: &req.Pricing.PayAmount.Amount, Currency: stringPtr(payment.NormalizeCurrency(req.Pricing.PayAmount.Currency))},
	}
}

func (c *Client) h5Request(req payment.CreatePaymentRequest) h5.PrepayRequest {
	return h5.PrepayRequest{
		Appid:       &c.appID,
		Mchid:       &c.merchantID,
		Description: stringPtr(description(req)),
		OutTradeNo:  stringPtr(outTradeNo(req)),
		TimeExpire:  req.ExpireAt,
		Attach:      optionalString(req.OrderID),
		NotifyUrl:   stringPtr(firstNonEmpty(req.NotifyURL, c.notifyURL)),
		Amount:      &h5.Amount{Total: &req.Pricing.PayAmount.Amount, Currency: stringPtr(payment.NormalizeCurrency(req.Pricing.PayAmount.Currency))},
	}
}

func (c *Client) nativeRequest(req payment.CreatePaymentRequest) native.PrepayRequest {
	return native.PrepayRequest{
		Appid:       &c.appID,
		Mchid:       &c.merchantID,
		Description: stringPtr(description(req)),
		OutTradeNo:  stringPtr(outTradeNo(req)),
		TimeExpire:  req.ExpireAt,
		Attach:      optionalString(req.OrderID),
		NotifyUrl:   stringPtr(firstNonEmpty(req.NotifyURL, c.notifyURL)),
		Amount:      &native.Amount{Total: &req.Pricing.PayAmount.Amount, Currency: stringPtr(payment.NormalizeCurrency(req.Pricing.PayAmount.Currency))},
	}
}

func newOfficialCoreClient(ctx context.Context, cfg Config) (*wechatcore.Client, error) {
	if strings.TrimSpace(cfg.MerchantSerialNo) == "" {
		return nil, fmt.Errorf("%w: wechat merchant serial no is required", payment.ErrInvalidRequest)
	}
	if cfg.PrivateKey == nil {
		return nil, fmt.Errorf("%w: wechat private key is required", payment.ErrInvalidRequest)
	}
	if strings.TrimSpace(cfg.APIv3Key) == "" {
		return nil, fmt.Errorf("%w: wechat api v3 key is required", payment.ErrInvalidRequest)
	}
	opts := []wechatcore.ClientOption{}
	if cfg.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(cfg.HTTPClient))
	}
	opts = append(opts, option.WithWechatPayAutoAuthCipher(cfg.MerchantID, cfg.MerchantSerialNo, cfg.PrivateKey, cfg.APIv3Key))
	return wechatcore.NewClient(ctx, opts...)
}

func servicesMissing(cfg Config) bool {
	return cfg.AppService == nil || cfg.JSAPIService == nil || cfg.H5Service == nil || cfg.NativeService == nil
}

func createResponse(req payment.CreatePaymentRequest, action payment.PaymentAction) *payment.CreatePaymentResponse {
	return &payment.CreatePaymentResponse{
		Provider:    payment.ProviderWechat,
		Channel:     req.Channel,
		MerchantKey: req.MerchantKey,
		PaymentID:   req.PaymentID,
		OrderID:     req.OrderID,
		OutTradeNo:  req.OutTradeNo,
		Status:      payment.PaymentPending,
		Pricing:     req.Pricing,
		Action:      action,
	}
}

func description(req payment.CreatePaymentRequest) string {
	return firstNonEmpty(req.Subject, req.Body, req.OutTradeNo, req.PaymentID)
}

func outTradeNo(req payment.CreatePaymentRequest) string {
	return firstNonEmpty(req.OutTradeNo, req.PaymentID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func stringExtra(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	value, ok := extra[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func stringPtr(value string) *string {
	return &value
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return stringPtr(strings.TrimSpace(value))
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
