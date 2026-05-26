package payment_test

import (
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
)

func TestStatusFromEvent(t *testing.T) {
	tests := []struct {
		name  string
		event payment.PaymentEvent
		want  payment.PaymentStatus
	}{
		{
			name: "explicit status wins",
			event: payment.PaymentEvent{
				Type:   payment.EventPaymentSucceeded,
				Status: payment.PaymentProcessing,
			},
			want: payment.PaymentProcessing,
		},
		{
			name:  "succeeded",
			event: payment.PaymentEvent{Type: payment.EventPaymentSucceeded},
			want:  payment.PaymentSucceeded,
		},
		{
			name:  "authorized",
			event: payment.PaymentEvent{Type: payment.EventPaymentAuthorized},
			want:  payment.PaymentAuthorized,
		},
		{
			name:  "refund processing",
			event: payment.PaymentEvent{Type: payment.EventPaymentRefunding},
			want:  payment.PaymentRefunding,
		},
		{
			name:  "refund event succeeded",
			event: payment.PaymentEvent{Type: payment.EventRefundSucceeded},
			want:  payment.PaymentRefunded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := payment.StatusFromEvent(tt.event); got != tt.want {
				t.Fatalf("status = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanTransitionPaymentStatus(t *testing.T) {
	tests := []struct {
		name string
		from payment.PaymentStatus
		to   payment.PaymentStatus
		want bool
	}{
		{name: "empty to pending", to: payment.PaymentPending, want: true},
		{name: "pending to succeeded", from: payment.PaymentPending, to: payment.PaymentSucceeded, want: true},
		{name: "closed to succeeded correction", from: payment.PaymentClosed, to: payment.PaymentSucceeded, want: true},
		{name: "succeeded to failed denied", from: payment.PaymentSucceeded, to: payment.PaymentFailed, want: false},
		{name: "refunded to succeeded denied", from: payment.PaymentRefunded, to: payment.PaymentSucceeded, want: false},
		{name: "succeeded to refunded", from: payment.PaymentSucceeded, to: payment.PaymentRefunded, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := payment.CanTransitionPaymentStatus(tt.from, tt.to); got != tt.want {
				t.Fatalf("transition %q -> %q = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestEventFromQueryPaymentResponse(t *testing.T) {
	event := payment.EventFromQueryPaymentResponse(&payment.QueryPaymentResponse{
		Provider:   payment.ProviderWechat,
		Channel:    payment.ChannelWechatApp,
		OutTradeNo: "pay_1001",
		Status:     payment.PaymentSucceeded,
		Pricing: payment.PaymentPricing{
			PayAmount: payment.Money{Amount: 100, Currency: "CNY"},
		},
	})
	if event == nil {
		t.Fatal("event is nil")
	}
	if event.Type != payment.EventPaymentSucceeded {
		t.Fatalf("event type = %q", event.Type)
	}
	if event.Amount != (payment.Money{Amount: 100, Currency: "CNY"}) {
		t.Fatalf("amount = %+v", event.Amount)
	}
}

func TestEventFromQueryPaymentResponseUsesPaymentRefundAggregateEvents(t *testing.T) {
	event := payment.EventFromQueryPaymentResponse(&payment.QueryPaymentResponse{
		Provider: payment.ProviderWechat,
		Status:   payment.PaymentPartialRefunded,
	})
	if event == nil {
		t.Fatal("event is nil")
	}
	if event.Type != payment.EventPaymentPartRefund {
		t.Fatalf("event type = %q, want payment partial refund event", event.Type)
	}
}

func TestEventFromQueryRefundResponse(t *testing.T) {
	event := payment.EventFromQueryRefundResponse(&payment.QueryRefundResponse{
		Provider: payment.ProviderWechat,
		RefundID: "refund_1",
		Status:   payment.RefundSucceeded,
		Amount: payment.RefundAmount{
			Refund: payment.Money{Amount: 100, Currency: "CNY"},
		},
	})
	if event == nil {
		t.Fatal("event is nil")
	}
	if event.Type != payment.EventRefundSucceeded {
		t.Fatalf("event type = %q", event.Type)
	}
	if event.RefundID != "refund_1" {
		t.Fatalf("refund id = %q", event.RefundID)
	}
	if event.RefundStatus != payment.RefundSucceeded {
		t.Fatalf("refund status = %q", event.RefundStatus)
	}
	if event.Amount != (payment.Money{Amount: 100, Currency: "CNY"}) {
		t.Fatalf("amount = %+v", event.Amount)
	}
}
