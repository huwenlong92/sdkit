package stripego

import (
	"strings"

	"github.com/huwenlong92/sdkit/core/payment"
	official "github.com/stripe/stripe-go/v85"
)

func checkoutPaymentStatus(session *official.CheckoutSession) payment.PaymentStatus {
	if session == nil {
		return payment.PaymentPending
	}
	switch strings.ToLower(string(session.PaymentStatus)) {
	case "paid", "no_payment_required":
		return payment.PaymentSucceeded
	case "unpaid":
		switch strings.ToLower(string(session.Status)) {
		case "expired":
			return payment.PaymentClosed
		case "complete":
			return payment.PaymentProcessing
		default:
			return payment.PaymentPending
		}
	default:
		switch strings.ToLower(string(session.Status)) {
		case "expired":
			return payment.PaymentClosed
		case "complete":
			return payment.PaymentProcessing
		default:
			return payment.PaymentPending
		}
	}
}

func intentPaymentStatus(intent *official.PaymentIntent) payment.PaymentStatus {
	if intent == nil {
		return payment.PaymentPending
	}
	switch strings.ToLower(string(intent.Status)) {
	case "succeeded":
		return payment.PaymentSucceeded
	case "processing":
		return payment.PaymentProcessing
	case "requires_action", "requires_confirmation":
		return payment.PaymentRequiresAction
	case "requires_capture":
		return payment.PaymentAuthorized
	case "canceled":
		return payment.PaymentClosed
	case "requires_payment_method":
		return payment.PaymentPending
	default:
		return payment.PaymentPending
	}
}

func refundStatus(refund *official.Refund) payment.RefundStatus {
	if refund == nil {
		return payment.RefundPending
	}
	switch strings.ToLower(string(refund.Status)) {
	case "succeeded":
		return payment.RefundSucceeded
	case "pending", "requires_action":
		return payment.RefundProcessing
	case "failed":
		return payment.RefundFailed
	case "canceled":
		return payment.RefundClosed
	default:
		return payment.RefundPending
	}
}

func refundResponse(req payment.RefundRequest, refund *official.Refund) *payment.RefundResponse {
	providerTradeID := req.ProviderTradeID
	if providerTradeID == "" && refund.PaymentIntent != nil {
		providerTradeID = refund.PaymentIntent.ID
	}
	return &payment.RefundResponse{
		Provider:         payment.ProviderStripe,
		Channel:          req.Channel,
		MerchantKey:      req.MerchantKey,
		PaymentID:        req.PaymentID,
		OrderID:          req.OrderID,
		OutTradeNo:       req.OutTradeNo,
		ProviderTradeID:  providerTradeID,
		RefundID:         req.RefundID,
		OutRefundNo:      req.OutRefundNo,
		ProviderRefundID: refund.ID,
		Status:           refundStatus(refund),
		Amount: payment.RefundAmount{
			Refund: payment.Money{
				Amount:   refund.Amount,
				Currency: payment.NormalizeCurrency(string(refund.Currency)),
			},
		},
		Raw: refund,
	}
}

func refundQueryResponse(req payment.QueryRefundRequest, refund *official.Refund) *payment.QueryRefundResponse {
	providerTradeID := req.ProviderTradeID
	if providerTradeID == "" && refund.PaymentIntent != nil {
		providerTradeID = refund.PaymentIntent.ID
	}
	return &payment.QueryRefundResponse{
		Provider:         payment.ProviderStripe,
		Channel:          req.Channel,
		MerchantKey:      req.MerchantKey,
		PaymentID:        req.PaymentID,
		OutTradeNo:       req.OutTradeNo,
		ProviderTradeID:  providerTradeID,
		RefundID:         req.RefundID,
		OutRefundNo:      req.OutRefundNo,
		ProviderRefundID: refund.ID,
		Status:           refundStatus(refund),
		Amount: payment.RefundAmount{
			Refund: payment.Money{
				Amount:   refund.Amount,
				Currency: payment.NormalizeCurrency(string(refund.Currency)),
			},
		},
		Raw: refund,
	}
}

func pricingFromIntent(defaultPricing payment.PaymentPricing, intent *official.PaymentIntent) payment.PaymentPricing {
	if intent == nil || intent.Amount == 0 {
		return defaultPricing
	}
	defaultPricing.PayAmount = payment.Money{
		Amount:   intent.Amount,
		Currency: payment.NormalizeCurrency(string(intent.Currency)),
	}
	return defaultPricing
}

func stripeRefundReason(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "duplicate", "fraudulent", "requested_by_customer":
		return strings.ToLower(strings.TrimSpace(reason))
	default:
		return ""
	}
}
