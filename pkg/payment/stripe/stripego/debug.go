//go:build sdkit_payment_stripe

package stripego

import (
	"context"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/debuglog"
)

func (c *Client) debug(ctx context.Context, event debuglog.Event) {
	if c.debugLogger == nil {
		return
	}
	if c.debugPayloadMode != debuglog.PayloadFull {
		event.Request = nil
		event.Response = nil
	}
	c.debugLogger.DebugPayment(ctx, event)
}

func createDebugEvent(stage debuglog.Stage, operation string, req payment.CreatePaymentRequest, providerTradeID string, request any, response any, err error) debuglog.Event {
	return debuglog.Event{
		Component:       "stripego",
		Stage:           stage,
		Operation:       operation,
		Provider:        payment.ProviderStripe,
		Channel:         req.Channel,
		MerchantKey:     req.MerchantKey,
		PaymentID:       req.PaymentID,
		OrderID:         req.OrderID,
		OutTradeNo:      req.OutTradeNo,
		ProviderTradeID: providerTradeID,
		Request:         request,
		Response:        response,
		Err:             err,
	}
}

func queryPaymentDebugEvent(stage debuglog.Stage, operation string, req payment.QueryPaymentRequest, providerTradeID string, request any, response any, err error) debuglog.Event {
	return debuglog.Event{
		Component:       "stripego",
		Stage:           stage,
		Operation:       operation,
		Provider:        payment.ProviderStripe,
		Channel:         req.Channel,
		MerchantKey:     req.MerchantKey,
		PaymentID:       req.PaymentID,
		OrderID:         req.OrderID,
		OutTradeNo:      req.OutTradeNo,
		ProviderTradeID: providerTradeID,
		Request:         request,
		Response:        response,
		Err:             err,
	}
}

func closePaymentDebugEvent(stage debuglog.Stage, operation string, req payment.ClosePaymentRequest, providerTradeID string, request any, response any, err error) debuglog.Event {
	return debuglog.Event{
		Component:       "stripego",
		Stage:           stage,
		Operation:       operation,
		Provider:        payment.ProviderStripe,
		Channel:         req.Channel,
		MerchantKey:     req.MerchantKey,
		PaymentID:       req.PaymentID,
		OrderID:         req.OrderID,
		OutTradeNo:      req.OutTradeNo,
		ProviderTradeID: providerTradeID,
		Request:         request,
		Response:        response,
		Err:             err,
	}
}

func refundRequestDebugEvent(stage debuglog.Stage, operation string, req payment.RefundRequest, request any, response any, err error) debuglog.Event {
	return debuglog.Event{
		Component:       "stripego",
		Stage:           stage,
		Operation:       operation,
		Provider:        payment.ProviderStripe,
		Channel:         req.Channel,
		MerchantKey:     req.MerchantKey,
		PaymentID:       req.PaymentID,
		OrderID:         req.OrderID,
		OutTradeNo:      req.OutTradeNo,
		ProviderTradeID: req.ProviderTradeID,
		RefundID:        req.RefundID,
		OutRefundNo:     req.OutRefundNo,
		Request:         request,
		Response:        response,
		Err:             err,
	}
}

func queryRefundRequestDebugEvent(stage debuglog.Stage, operation string, req payment.QueryRefundRequest, providerRefundID string, request any, response any, err error) debuglog.Event {
	return debuglog.Event{
		Component:        "stripego",
		Stage:            stage,
		Operation:        operation,
		Provider:         payment.ProviderStripe,
		Channel:          req.Channel,
		MerchantKey:      req.MerchantKey,
		PaymentID:        req.PaymentID,
		OutTradeNo:       req.OutTradeNo,
		ProviderTradeID:  req.ProviderTradeID,
		RefundID:         req.RefundID,
		OutRefundNo:      req.OutRefundNo,
		ProviderRefundID: providerRefundID,
		Request:          request,
		Response:         response,
		Err:              err,
	}
}
