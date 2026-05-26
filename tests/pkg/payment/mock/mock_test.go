package mock_test

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	paymentmock "github.com/huwenlong92/sdkit/pkg/payment/mock"
)

func TestMockAdapterWorksWithServiceCreatePayment(t *testing.T) {
	adapter := paymentmock.New(
		payment.ProviderAggregate,
		paymentmock.WithChannels(payment.ChannelAggregateForm),
		paymentmock.WithCurrencies("CNY"),
		paymentmock.WithActions(payment.ActionHTMLForm),
		paymentmock.WithAction(payment.PaymentAction{
			Type:   payment.ActionHTMLForm,
			URL:    "https://pay.example.test/form",
			Fields: map[string]string{"token": "abc"},
		}),
	)
	registry := payment.NewRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc, err := payment.NewService(payment.ServiceConfig{Registry: registry})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	resp, err := svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderAggregate,
		Channel:    payment.ChannelAggregateForm,
		OutTradeNo: "pay_1001",
		Pricing:    payment.CNY(100),
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if resp.Action.Type != payment.ActionHTMLForm || resp.Action.Method != "POST" {
		t.Fatalf("action = %+v", resp.Action)
	}
	snapshot := adapter.Snapshot()
	if len(snapshot.CreateRequests) != 1 {
		t.Fatalf("create requests = %d", len(snapshot.CreateRequests))
	}
	if snapshot.CreateRequests[0].Pricing.PayAmount.Currency != "CNY" {
		t.Fatalf("pricing = %+v", snapshot.CreateRequests[0].Pricing)
	}
}

func TestMockAdapterNotify(t *testing.T) {
	adapter := paymentmock.New(
		payment.ProviderWechat,
		paymentmock.WithChannels(payment.ChannelWechatApp),
		paymentmock.WithAllCoreCapabilities(),
		paymentmock.WithParseNotify(func(_ context.Context, req payment.NotifyRequest) (*payment.NotifyResult, error) {
			return &payment.NotifyResult{
				Verified: true,
				Event: &payment.PaymentEvent{
					Type:       payment.EventPaymentSucceeded,
					Provider:   req.Provider,
					Channel:    req.Channel,
					OutTradeNo: "pay_1001",
					Status:     payment.PaymentSucceeded,
				},
				Ack: payment.NotifyAck{StatusCode: 200, Body: []byte("ok")},
			}, nil
		}),
	)
	registry := payment.NewRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc, err := payment.NewService(payment.ServiceConfig{Registry: registry})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := svc.HandleNotify(context.Background(), payment.NotifyRequest{
		Provider: payment.ProviderWechat,
		Channel:  payment.ChannelWechatApp,
		Body:     []byte(`{"id":"evt_1"}`),
	})
	if err != nil {
		t.Fatalf("handle notify: %v", err)
	}
	if !result.Verified || result.Event == nil || result.Event.OutTradeNo != "pay_1001" {
		t.Fatalf("notify result = %+v", result)
	}
	if got := len(adapter.Snapshot().NotifyRequests); got != 1 {
		t.Fatalf("notify requests = %d", got)
	}
}

func TestMockAdapterRefundAndQueryRefund(t *testing.T) {
	adapter := paymentmock.New(
		payment.ProviderWechat,
		paymentmock.WithChannels(payment.ChannelWechatApp),
		paymentmock.WithCurrencies("CNY"),
		paymentmock.WithAllCoreCapabilities(),
	)
	registry := payment.NewRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc, err := payment.NewService(payment.ServiceConfig{Registry: registry})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	refundResp, err := svc.Refund(context.Background(), payment.RefundRequest{
		Provider:  payment.ProviderWechat,
		Channel:   payment.ChannelWechatApp,
		PaymentID: "pay_1001",
		RefundID:  "refund_1001",
		Amount: payment.RefundAmount{
			Refund: payment.Money{Amount: 50},
		},
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if refundResp.Amount.Refund.Currency != "CNY" {
		t.Fatalf("refund resp = %+v", refundResp)
	}

	queryResp, err := svc.QueryRefund(context.Background(), payment.QueryRefundRequest{
		Provider: payment.ProviderWechat,
		Channel:  payment.ChannelWechatApp,
		RefundID: "refund_1001",
	})
	if err != nil {
		t.Fatalf("query refund: %v", err)
	}
	if queryResp.Status != payment.RefundSucceeded {
		t.Fatalf("query refund status = %q", queryResp.Status)
	}
}
