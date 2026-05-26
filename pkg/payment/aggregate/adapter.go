package aggregate

import (
	"context"
	"fmt"
	"strings"

	"github.com/huwenlong92/sdkit/core/payment"
)

type Adapter struct {
	defaultGateway string
	gateways       map[string]Gateway
	capabilities   payment.Capabilities
}

func New(cfg Config) (*Adapter, error) {
	gateways := make(map[string]Gateway, len(cfg.Gateways))
	for key, gateway := range cfg.Gateways {
		key = strings.TrimSpace(key)
		if key == "" || gateway == nil {
			continue
		}
		gateways[key] = gateway
	}
	if len(gateways) == 0 {
		return nil, fmt.Errorf("%w: aggregate gateway is required", payment.ErrAdapterNotFound)
	}

	defaultGateway := strings.TrimSpace(cfg.DefaultGateway)
	if defaultGateway == "" && len(gateways) == 1 {
		for key := range gateways {
			defaultGateway = key
		}
	}
	if defaultGateway != "" {
		if _, ok := gateways[defaultGateway]; !ok {
			return nil, fmt.Errorf("%w: aggregate gateway %s", payment.ErrAdapterNotFound, defaultGateway)
		}
	}

	return &Adapter{
		defaultGateway: defaultGateway,
		gateways:       gateways,
		capabilities:   normalizeCapabilities(cfg.Capabilities),
	}, nil
}

func (a *Adapter) Name() payment.Provider {
	return payment.ProviderAggregate
}

func (a *Adapter) Capabilities() payment.Capabilities {
	return a.capabilities
}

func (a *Adapter) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	gateway, err := a.gateway(req.MerchantKey, req.Extra)
	if err != nil {
		return nil, err
	}
	return gateway.CreatePayment(ctx, req)
}

func (a *Adapter) QueryPayment(ctx context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	gateway, err := a.gateway(req.MerchantKey, req.Extra)
	if err != nil {
		return nil, err
	}
	queryGateway, ok := gateway.(QueryGateway)
	if !ok {
		return nil, fmt.Errorf("%w: aggregate query payment", payment.ErrUnsupportedCapability)
	}
	return queryGateway.QueryPayment(ctx, req)
}

func (a *Adapter) ClosePayment(ctx context.Context, req payment.ClosePaymentRequest) error {
	if ctx == nil {
		return payment.ErrNilContext
	}
	gateway, err := a.gateway(req.MerchantKey, req.Extra)
	if err != nil {
		return err
	}
	closeGateway, ok := gateway.(CloseGateway)
	if !ok {
		return fmt.Errorf("%w: aggregate close payment", payment.ErrUnsupportedCapability)
	}
	return closeGateway.ClosePayment(ctx, req)
}

func (a *Adapter) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	gateway, err := a.gateway(req.MerchantKey, req.Extra)
	if err != nil {
		return nil, err
	}
	refundGateway, ok := gateway.(RefundGateway)
	if !ok {
		return nil, fmt.Errorf("%w: aggregate refund", payment.ErrUnsupportedCapability)
	}
	return refundGateway.Refund(ctx, req)
}

func (a *Adapter) QueryRefund(ctx context.Context, req payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	gateway, err := a.gateway(req.MerchantKey, req.Extra)
	if err != nil {
		return nil, err
	}
	queryRefundGateway, ok := gateway.(QueryRefundGateway)
	if !ok {
		return nil, fmt.Errorf("%w: aggregate query refund", payment.ErrUnsupportedCapability)
	}
	return queryRefundGateway.QueryRefund(ctx, req)
}

func (a *Adapter) ParseNotify(ctx context.Context, req payment.NotifyRequest) (*payment.NotifyResult, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	gateway, err := a.gateway("", notifyExtra(req))
	if err != nil {
		return nil, err
	}
	notifyGateway, ok := gateway.(NotifyGateway)
	if !ok {
		return nil, fmt.Errorf("%w: aggregate notify", payment.ErrUnsupportedCapability)
	}
	return notifyGateway.ParseNotify(ctx, req)
}

func (a *Adapter) gateway(merchantKey string, extra map[string]any) (Gateway, error) {
	if merchantKey = strings.TrimSpace(merchantKey); merchantKey != "" {
		if gateway, ok := a.gateways[merchantKey]; ok {
			return gateway, nil
		}
	}
	if key := gatewayKeyFromExtra(extra); key != "" {
		if gateway, ok := a.gateways[key]; ok {
			return gateway, nil
		}
		return nil, fmt.Errorf("%w: aggregate gateway %s", payment.ErrAdapterNotFound, key)
	}
	if a.defaultGateway != "" {
		return a.gateways[a.defaultGateway], nil
	}
	if merchantKey != "" {
		return nil, fmt.Errorf("%w: aggregate gateway %s", payment.ErrAdapterNotFound, merchantKey)
	}
	return nil, fmt.Errorf("%w: aggregate gateway is required", payment.ErrAdapterNotFound)
}

func normalizeCapabilities(caps payment.Capabilities) payment.Capabilities {
	if caps.Provider == "" {
		caps.Provider = payment.ProviderAggregate
	}
	if len(caps.Channels) == 0 {
		caps.Channels = []payment.Channel{payment.ChannelAggregateForm}
	}
	if len(caps.SupportedCurrencies) == 0 {
		caps.SupportedCurrencies = []string{payment.DefaultCurrency}
	}
	if len(caps.SupportedActions) == 0 {
		caps.SupportedActions = []payment.ActionType{
			payment.ActionHTMLForm,
			payment.ActionRedirectURL,
			payment.ActionQRCode,
			payment.ActionSDKParams,
			payment.ActionClientToken,
			payment.ActionNone,
		}
	}
	caps.SupportsMultiMerchant = true
	return caps
}

func gatewayKeyFromExtra(extra map[string]any) string {
	if extra == nil {
		return ""
	}
	raw, ok := extra[ExtraGatewayKey]
	if !ok {
		return ""
	}
	key, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(key)
}

func notifyExtra(req payment.NotifyRequest) map[string]any {
	if key := firstQueryValue(req.Query, ExtraGatewayKey); key != "" {
		return map[string]any{ExtraGatewayKey: key}
	}
	if key := firstQueryValue(req.Form, ExtraGatewayKey); key != "" {
		return map[string]any{ExtraGatewayKey: key}
	}
	return nil
}

func firstQueryValue(values map[string][]string, key string) string {
	if len(values) == 0 {
		return ""
	}
	items := values[key]
	if len(items) == 0 {
		return ""
	}
	return strings.TrimSpace(items[0])
}
