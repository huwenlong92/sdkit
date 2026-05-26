package channelrouter

import "github.com/huwenlong92/sdkit/core/payment"

func createDebugEvent(stage DebugStage, op Operation, req payment.CreatePaymentRequest, route *ResolvedRoute, err error) DebugEvent {
	event := DebugEvent{
		Stage:                stage,
		Operation:            op,
		Provider:             req.Provider,
		Channel:              req.Channel,
		RequestedMerchantKey: req.MerchantKey,
		PaymentID:            req.PaymentID,
		OrderID:              req.OrderID,
		OutTradeNo:           req.OutTradeNo,
		Err:                  err,
	}
	if route != nil {
		event.ResolvedMerchantKey = route.MerchantKey
	}
	return event
}

func createResponseDebugEvent(stage DebugStage, op Operation, req payment.CreatePaymentRequest, resp *payment.CreatePaymentResponse, route *ResolvedRoute, err error) DebugEvent {
	event := createDebugEvent(stage, op, req, route, err)
	if resp != nil {
		event.ProviderTradeID = resp.ProviderTradeID
	}
	return event
}

func queryDebugEvent(stage DebugStage, op Operation, req payment.QueryPaymentRequest, route *ResolvedRoute, err error) DebugEvent {
	event := DebugEvent{
		Stage:                stage,
		Operation:            op,
		Provider:             req.Provider,
		Channel:              req.Channel,
		RequestedMerchantKey: req.MerchantKey,
		PaymentID:            req.PaymentID,
		OrderID:              req.OrderID,
		OutTradeNo:           req.OutTradeNo,
		ProviderTradeID:      req.ProviderTradeID,
		Err:                  err,
	}
	if route != nil {
		event.ResolvedMerchantKey = route.MerchantKey
	}
	return event
}

func queryResponseDebugEvent(stage DebugStage, op Operation, req payment.QueryPaymentRequest, resp *payment.QueryPaymentResponse, route *ResolvedRoute, err error) DebugEvent {
	event := queryDebugEvent(stage, op, req, route, err)
	if resp != nil {
		event.ProviderTradeID = resp.ProviderTradeID
	}
	return event
}

func closeDebugEvent(stage DebugStage, op Operation, req payment.ClosePaymentRequest, route *ResolvedRoute, err error) DebugEvent {
	event := DebugEvent{
		Stage:                stage,
		Operation:            op,
		Provider:             req.Provider,
		Channel:              req.Channel,
		RequestedMerchantKey: req.MerchantKey,
		PaymentID:            req.PaymentID,
		OrderID:              req.OrderID,
		OutTradeNo:           req.OutTradeNo,
		ProviderTradeID:      req.ProviderTradeID,
		Err:                  err,
	}
	if route != nil {
		event.ResolvedMerchantKey = route.MerchantKey
	}
	return event
}

func refundDebugEvent(stage DebugStage, op Operation, req payment.RefundRequest, route *ResolvedRoute, err error) DebugEvent {
	event := DebugEvent{
		Stage:                stage,
		Operation:            op,
		Provider:             req.Provider,
		Channel:              req.Channel,
		RequestedMerchantKey: req.MerchantKey,
		PaymentID:            req.PaymentID,
		OrderID:              req.OrderID,
		OutTradeNo:           req.OutTradeNo,
		ProviderTradeID:      req.ProviderTradeID,
		RefundID:             req.RefundID,
		OutRefundNo:          req.OutRefundNo,
		Err:                  err,
	}
	if route != nil {
		event.ResolvedMerchantKey = route.MerchantKey
	}
	return event
}

func refundResponseDebugEvent(stage DebugStage, op Operation, req payment.RefundRequest, resp *payment.RefundResponse, route *ResolvedRoute, err error) DebugEvent {
	event := refundDebugEvent(stage, op, req, route, err)
	if resp != nil {
		event.ProviderRefundID = resp.ProviderRefundID
		event.ProviderTradeID = resp.ProviderTradeID
	}
	return event
}

func queryRefundDebugEvent(stage DebugStage, op Operation, req payment.QueryRefundRequest, route *ResolvedRoute, err error) DebugEvent {
	event := DebugEvent{
		Stage:                stage,
		Operation:            op,
		Provider:             req.Provider,
		Channel:              req.Channel,
		RequestedMerchantKey: req.MerchantKey,
		PaymentID:            req.PaymentID,
		OutTradeNo:           req.OutTradeNo,
		ProviderTradeID:      req.ProviderTradeID,
		RefundID:             req.RefundID,
		OutRefundNo:          req.OutRefundNo,
		ProviderRefundID:     req.ProviderRefundID,
		Err:                  err,
	}
	if route != nil {
		event.ResolvedMerchantKey = route.MerchantKey
	}
	return event
}

func queryRefundResponseDebugEvent(stage DebugStage, op Operation, req payment.QueryRefundRequest, resp *payment.QueryRefundResponse, route *ResolvedRoute, err error) DebugEvent {
	event := queryRefundDebugEvent(stage, op, req, route, err)
	if resp != nil {
		event.ProviderRefundID = resp.ProviderRefundID
		event.ProviderTradeID = resp.ProviderTradeID
	}
	return event
}
