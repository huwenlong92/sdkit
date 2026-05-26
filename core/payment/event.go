package payment

func EventFromQueryPaymentResponse(resp *QueryPaymentResponse) *PaymentEvent {
	if resp == nil || resp.Status == "" {
		return nil
	}
	eventType := EventUnknown
	switch resp.Status {
	case PaymentSucceeded:
		eventType = EventPaymentSucceeded
	case PaymentFailed:
		eventType = EventPaymentFailed
	case PaymentClosed:
		eventType = EventPaymentClosed
	case PaymentAuthorized:
		eventType = EventPaymentAuthorized
	case PaymentRefunding:
		eventType = EventPaymentRefunding
	case PaymentPartialRefunded:
		eventType = EventPaymentPartRefund
	case PaymentRefunded:
		eventType = EventPaymentRefunded
	}
	return &PaymentEvent{
		Type:            eventType,
		Provider:        resp.Provider,
		Channel:         resp.Channel,
		MerchantKey:     resp.MerchantKey,
		PaymentID:       resp.PaymentID,
		OrderID:         resp.OrderID,
		OutTradeNo:      resp.OutTradeNo,
		ProviderTradeID: resp.ProviderTradeID,
		Status:          resp.Status,
		Amount:          resp.Pricing.PayAmount,
		PaidAt:          resp.PaidAt,
	}
}

func EventFromQueryRefundResponse(resp *QueryRefundResponse) *PaymentEvent {
	if resp == nil || resp.Status == "" {
		return nil
	}
	eventType := EventUnknown
	switch resp.Status {
	case RefundPending:
		eventType = EventRefundCreated
	case RefundProcessing:
		eventType = EventRefundProcessing
	case RefundSucceeded:
		eventType = EventRefundSucceeded
	case RefundFailed, RefundClosed:
		eventType = EventRefundFailed
	}
	return &PaymentEvent{
		Type:             eventType,
		Provider:         resp.Provider,
		Channel:          resp.Channel,
		MerchantKey:      resp.MerchantKey,
		PaymentID:        resp.PaymentID,
		RefundID:         resp.RefundID,
		ProviderRefundID: resp.ProviderRefundID,
		RefundStatus:     resp.Status,
		Amount:           resp.Amount.Refund,
	}
}
