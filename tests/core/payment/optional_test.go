package payment_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
)

func TestOptionalSettlementAndPayout(t *testing.T) {
	pricing, err := payment.NormalizePricing(context.Background(), payment.PaymentPricing{PayAmount: payment.Money{Amount: 10000, Currency: "USD"}})
	if err != nil || pricing.SettleCurrency != "" || pricing.SettleAmount != (payment.Money{}) {
		t.Fatalf("USD without settlement: %+v %v", pricing, err)
	}
	settlement := &payment.ProviderSettlement{Currency: "CNY", FeeAmount: &payment.Money{Currency: "CNY", Amount: 0}}
	event := payment.EventFromQueryPaymentResponse(&payment.QueryPaymentResponse{Status: payment.PaymentSucceeded, Pricing: pricing, ProviderSettlement: settlement})
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded payment.PaymentEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ProviderSettlement == nil || decoded.ProviderSettlement.Amount != nil || decoded.ProviderSettlement.FeeAmount == nil || decoded.ProviderSettlement.FeeAmount.Amount != 0 {
		t.Fatalf("optional fields lost: %s", raw)
	}
	registry := payment.NewRegistry()
	if err := registry.Register(&serviceAdapter{name: payment.ProviderWechat}); err != nil {
		t.Fatal(err)
	}
	svc, err := payment.NewService(payment.ServiceConfig{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreatePayout(context.Background(), payment.CreatePayoutRequest{Provider: payment.ProviderWechat}); !errors.Is(err, payment.ErrUnsupportedCapability) {
		t.Fatalf("unsupported payout: %v", err)
	}
}

func TestRuntimeRegistrationAndMerchantSwitch(t *testing.T) {
	registry := payment.NewRegistry()
	selector, err := payment.NewStaticChannelSelector([]payment.ChannelBinding{{Key: "old", Provider: payment.ProviderWechat, Channel: payment.ChannelWechatNative, MerchantKey: "old"}, {Key: "new", Provider: payment.ProviderWechat, Channel: payment.ChannelWechatNative, MerchantKey: "new"}})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := payment.NewService(payment.ServiceConfig{Registry: registry, ChannelSelector: selector})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &serviceAdapter{name: payment.ProviderWechat, caps: payment.Capabilities{SupportsQuery: true}, actionType: payment.ActionQRCode}
	if err := svc.RegisterProvider(adapter); err != nil {
		t.Fatal(err)
	}
	if err := svc.RegisterProvider(adapter); !errors.Is(err, payment.ErrAdapterAlreadyExists) {
		t.Fatalf("duplicate provider %v", err)
	}
	if err := svc.ReloadChannels([]payment.ChannelBinding{{Key: "old", Provider: payment.ProviderWechat, Channel: payment.ChannelWechatNative, MerchantKey: "old", DisabledForNew: true}, {Key: "new", Provider: payment.ProviderWechat, Channel: payment.ChannelWechatNative, MerchantKey: "new"}}); err != nil {
		t.Fatal(err)
	}
	req := payment.CreatePaymentRequest{MerchantKey: "old", OutTradeNo: "order", Pricing: payment.PaymentPricing{PayAmount: payment.Money{Amount: 100, Currency: "USD"}}}
	if _, err := svc.CreatePayment(context.Background(), req); !errors.Is(err, payment.ErrInvalidRequest) {
		t.Fatalf("old merchant allowed new create: %v", err)
	}
	req.MerchantKey = "new"
	if _, err := svc.CreatePayment(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if adapter.createReq.MerchantKey != "new" {
		t.Fatal("wrong merchant selected")
	}
	if _, err := svc.QueryPayment(context.Background(), payment.QueryPaymentRequest{MerchantKey: "old", OutTradeNo: "order"}); err != nil {
		t.Fatalf("old lookup blocked %v", err)
	}
}
