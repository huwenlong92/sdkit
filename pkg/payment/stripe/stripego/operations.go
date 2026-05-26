package stripego

import (
	"context"
	"fmt"
	"strings"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/debuglog"
	official "github.com/stripe/stripe-go/v85"
)

func (c *Client) QueryPayment(ctx context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	switch req.Channel {
	case payment.ChannelStripeCheckout:
		return c.queryCheckoutSession(ctx, req)
	case payment.ChannelStripeIntent:
		return c.queryPaymentIntent(ctx, req)
	default:
		return nil, fmt.Errorf("%w: stripe channel %s", payment.ErrUnsupportedChannel, req.Channel)
	}
}

func (c *Client) ClosePayment(ctx context.Context, req payment.ClosePaymentRequest) error {
	if ctx == nil {
		return payment.ErrNilContext
	}
	switch req.Channel {
	case payment.ChannelStripeCheckout:
		if c.checkoutService == nil {
			return fmt.Errorf("%w: stripe checkout service is required", payment.ErrAdapterNotFound)
		}
		id := firstNonEmpty(req.ProviderTradeID, stringExtra(req.Extra, ExtraCheckoutSessionIDKey))
		if id == "" {
			return fmt.Errorf("%w: stripe checkout session id is required", payment.ErrPaymentReference)
		}
		params := &official.CheckoutSessionExpireParams{}
		c.debug(ctx, closePaymentDebugEvent(debuglog.StageRequest, "expire_checkout_session", req, id, params, nil, nil))
		_, err := c.checkoutService.ExpireCheckoutSession(ctx, id, params)
		if err != nil {
			c.debug(ctx, closePaymentDebugEvent(debuglog.StageError, "expire_checkout_session", req, id, params, nil, err))
		}
		return err
	case payment.ChannelStripeIntent:
		if c.paymentIntentService == nil {
			return fmt.Errorf("%w: stripe payment intent service is required", payment.ErrAdapterNotFound)
		}
		id := firstNonEmpty(req.ProviderTradeID, stringExtra(req.Extra, ExtraPaymentIntentIDKey))
		if id == "" {
			return fmt.Errorf("%w: stripe payment intent id is required", payment.ErrPaymentReference)
		}
		params := &official.PaymentIntentCancelParams{}
		c.debug(ctx, closePaymentDebugEvent(debuglog.StageRequest, "cancel_payment_intent", req, id, params, nil, nil))
		resp, err := c.paymentIntentService.CancelPaymentIntent(ctx, id, params)
		if err != nil {
			c.debug(ctx, closePaymentDebugEvent(debuglog.StageError, "cancel_payment_intent", req, id, params, nil, err))
			return err
		}
		c.debug(ctx, closePaymentDebugEvent(debuglog.StageResponse, "cancel_payment_intent", req, id, nil, resp, nil))
		return err
	default:
		return fmt.Errorf("%w: stripe channel %s", payment.ErrUnsupportedChannel, req.Channel)
	}
}

func (c *Client) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	if c.refundService == nil {
		return nil, fmt.Errorf("%w: stripe refund service is required", payment.ErrAdapterNotFound)
	}
	paymentIntentID := firstNonEmpty(req.ProviderTradeID, stringExtra(req.Extra, ExtraPaymentIntentIDKey))
	chargeID := stringExtra(req.Extra, ExtraChargeIDKey)
	if paymentIntentID == "" && chargeID == "" {
		return nil, fmt.Errorf("%w: stripe payment_intent or charge id is required", payment.ErrPaymentReference)
	}
	currency := stripeCurrency(req.Amount.Refund.Currency)
	params := &official.RefundCreateParams{
		Amount:        &req.Amount.Refund.Amount,
		Currency:      &currency,
		PaymentIntent: optionalString(paymentIntentID),
		Charge:        optionalString(chargeID),
		Reason:        optionalString(stripeRefundReason(req.Reason)),
		Metadata: map[string]string{
			"payment_id":    req.PaymentID,
			"order_id":      req.OrderID,
			"out_trade_no":  req.OutTradeNo,
			"refund_id":     req.RefundID,
			"out_refund_no": req.OutRefundNo,
		},
	}
	if params.Reason == nil && strings.TrimSpace(req.Reason) != "" {
		params.Metadata["reason_text"] = strings.TrimSpace(req.Reason)
	}
	params.Metadata = compactMetadata(params.Metadata)

	c.debug(ctx, refundRequestDebugEvent(debuglog.StageRequest, "create_refund", req, params, nil, nil))
	refund, err := c.refundService.CreateRefund(ctx, params)
	if err != nil {
		c.debug(ctx, refundRequestDebugEvent(debuglog.StageError, "create_refund", req, params, nil, err))
		return nil, err
	}
	if refund == nil || strings.TrimSpace(refund.ID) == "" {
		err := fmt.Errorf("%w: stripe refund id is required", payment.ErrPaymentActionRequired)
		c.debug(ctx, refundRequestDebugEvent(debuglog.StageError, "create_refund", req, params, refund, err))
		return nil, err
	}
	c.debug(ctx, refundRequestDebugEvent(debuglog.StageResponse, "create_refund", req, nil, refund, nil))
	return refundResponse(req, refund), nil
}

func (c *Client) QueryRefund(ctx context.Context, req payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	if c.refundService == nil {
		return nil, fmt.Errorf("%w: stripe refund service is required", payment.ErrAdapterNotFound)
	}
	id := firstNonEmpty(req.ProviderRefundID, req.RefundID)
	if id == "" {
		return nil, fmt.Errorf("%w: stripe refund id is required", payment.ErrPaymentReference)
	}
	params := &official.RefundRetrieveParams{}
	c.debug(ctx, queryRefundRequestDebugEvent(debuglog.StageRequest, "retrieve_refund", req, id, params, nil, nil))
	refund, err := c.refundService.RetrieveRefund(ctx, id, params)
	if err != nil {
		c.debug(ctx, queryRefundRequestDebugEvent(debuglog.StageError, "retrieve_refund", req, id, params, nil, err))
		return nil, err
	}
	if refund == nil {
		err := fmt.Errorf("%w: nil stripe refund response", payment.ErrPaymentActionRequired)
		c.debug(ctx, queryRefundRequestDebugEvent(debuglog.StageError, "retrieve_refund", req, id, params, refund, err))
		return nil, err
	}
	c.debug(ctx, queryRefundRequestDebugEvent(debuglog.StageResponse, "retrieve_refund", req, id, nil, refund, nil))
	resp := refundQueryResponse(req, refund)
	return resp, nil
}

func (c *Client) queryCheckoutSession(ctx context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	if c.checkoutService == nil {
		return nil, fmt.Errorf("%w: stripe checkout service is required", payment.ErrAdapterNotFound)
	}
	id := firstNonEmpty(req.ProviderTradeID, stringExtra(req.Extra, ExtraCheckoutSessionIDKey))
	if id == "" {
		return nil, fmt.Errorf("%w: stripe checkout session id is required", payment.ErrPaymentReference)
	}
	params := &official.CheckoutSessionRetrieveParams{}
	c.debug(ctx, queryPaymentDebugEvent(debuglog.StageRequest, "retrieve_checkout_session", req, id, params, nil, nil))
	session, err := c.checkoutService.RetrieveCheckoutSession(ctx, id, params)
	if err != nil {
		c.debug(ctx, queryPaymentDebugEvent(debuglog.StageError, "retrieve_checkout_session", req, id, params, nil, err))
		return nil, err
	}
	if session == nil {
		err := fmt.Errorf("%w: nil stripe checkout session response", payment.ErrPaymentActionRequired)
		c.debug(ctx, queryPaymentDebugEvent(debuglog.StageError, "retrieve_checkout_session", req, id, params, session, err))
		return nil, err
	}
	c.debug(ctx, queryPaymentDebugEvent(debuglog.StageResponse, "retrieve_checkout_session", req, session.ID, nil, session, nil))
	extra := map[string]any{ExtraCheckoutSessionIDKey: session.ID}
	if session.PaymentIntent != nil && session.PaymentIntent.ID != "" {
		extra[ExtraPaymentIntentIDKey] = session.PaymentIntent.ID
	}
	return &payment.QueryPaymentResponse{
		Provider:        payment.ProviderStripe,
		Channel:         payment.ChannelStripeCheckout,
		MerchantKey:     req.MerchantKey,
		PaymentID:       req.PaymentID,
		OrderID:         req.OrderID,
		OutTradeNo:      req.OutTradeNo,
		ProviderTradeID: session.ID,
		Status:          checkoutPaymentStatus(session),
		Pricing: payment.PaymentPricing{
			PayAmount: payment.Money{
				Amount:   session.AmountTotal,
				Currency: payment.NormalizeCurrency(string(session.Currency)),
			},
		},
		Raw:   session,
		Extra: extra,
	}, nil
}

func (c *Client) queryPaymentIntent(ctx context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	if c.paymentIntentService == nil {
		return nil, fmt.Errorf("%w: stripe payment intent service is required", payment.ErrAdapterNotFound)
	}
	id := firstNonEmpty(req.ProviderTradeID, stringExtra(req.Extra, ExtraPaymentIntentIDKey))
	if id == "" {
		return nil, fmt.Errorf("%w: stripe payment intent id is required", payment.ErrPaymentReference)
	}
	params := &official.PaymentIntentRetrieveParams{}
	c.debug(ctx, queryPaymentDebugEvent(debuglog.StageRequest, "retrieve_payment_intent", req, id, params, nil, nil))
	intent, err := c.paymentIntentService.RetrievePaymentIntent(ctx, id, params)
	if err != nil {
		c.debug(ctx, queryPaymentDebugEvent(debuglog.StageError, "retrieve_payment_intent", req, id, params, nil, err))
		return nil, err
	}
	if intent == nil {
		err := fmt.Errorf("%w: nil stripe payment intent response", payment.ErrPaymentActionRequired)
		c.debug(ctx, queryPaymentDebugEvent(debuglog.StageError, "retrieve_payment_intent", req, id, params, intent, err))
		return nil, err
	}
	c.debug(ctx, queryPaymentDebugEvent(debuglog.StageResponse, "retrieve_payment_intent", req, intent.ID, nil, intent, nil))
	return &payment.QueryPaymentResponse{
		Provider:        payment.ProviderStripe,
		Channel:         payment.ChannelStripeIntent,
		MerchantKey:     req.MerchantKey,
		PaymentID:       req.PaymentID,
		OrderID:         req.OrderID,
		OutTradeNo:      req.OutTradeNo,
		ProviderTradeID: intent.ID,
		Status:          intentPaymentStatus(intent),
		Pricing:         pricingFromIntent(payment.PaymentPricing{}, intent),
		Raw:             intent,
		Extra: map[string]any{
			ExtraPaymentIntentIDKey: intent.ID,
		},
	}, nil
}
