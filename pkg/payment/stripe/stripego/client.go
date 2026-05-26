package stripego

import (
	"context"
	"fmt"
	"strings"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/debuglog"
	official "github.com/stripe/stripe-go/v85"
)

func NewClient(cfg Config) (*Client, error) {
	services := officialServices{}
	if cfg.OfficialClient != nil {
		services.client = cfg.OfficialClient
	}
	if services.client == nil && servicesMissing(cfg) {
		if strings.TrimSpace(cfg.APIKey) == "" {
			return nil, fmt.Errorf("%w: stripe api key is required", payment.ErrInvalidRequest)
		}
		services.client = official.NewClient(strings.TrimSpace(cfg.APIKey))
	}

	client := &Client{
		successURL:           strings.TrimSpace(cfg.SuccessURL),
		cancelURL:            strings.TrimSpace(cfg.CancelURL),
		checkoutService:      cfg.CheckoutService,
		paymentIntentService: cfg.PaymentIntentService,
		refundService:        cfg.RefundService,
		debugLogger:          cfg.DebugLogger,
		debugPayloadMode:     cfg.DebugPayloadMode,
	}
	if services.client != nil {
		if client.checkoutService == nil {
			client.checkoutService = services
		}
		if client.paymentIntentService == nil {
			client.paymentIntentService = services
		}
		if client.refundService == nil {
			client.refundService = services
		}
	}
	return client, nil
}

func (c *Client) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	switch req.Channel {
	case payment.ChannelStripeCheckout:
		return c.createCheckoutSession(ctx, req)
	case payment.ChannelStripeIntent:
		return c.createPaymentIntent(ctx, req)
	default:
		return nil, fmt.Errorf("%w: stripe channel %s", payment.ErrUnsupportedChannel, req.Channel)
	}
}

func (c *Client) createCheckoutSession(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if c.checkoutService == nil {
		return nil, fmt.Errorf("%w: stripe checkout service is required", payment.ErrAdapterNotFound)
	}
	successURL := firstNonEmpty(req.ReturnURL, c.successURL)
	if successURL == "" {
		return nil, fmt.Errorf("%w: stripe success url is required", payment.ErrInvalidRequest)
	}
	cancelURL := firstNonEmpty(stringExtra(req.Extra, ExtraCancelURLKey), c.cancelURL, successURL)
	meta := metadata(req)
	currency := stripeCurrency(req.Pricing.PayAmount.Currency)
	quantity := int64(1)
	mode := "payment"
	description := description(req)

	params := &official.CheckoutSessionCreateParams{
		Mode:              &mode,
		SuccessURL:        &successURL,
		CancelURL:         &cancelURL,
		ClientReferenceID: optionalString(reference(req)),
		Customer:          optionalString(stringExtra(req.Extra, ExtraCustomerIDKey)),
		CustomerEmail:     optionalString(stringExtra(req.Extra, ExtraReceiptEmailKey)),
		Metadata:          meta,
		PaymentIntentData: &official.CheckoutSessionCreatePaymentIntentDataParams{
			Description:  optionalString(description),
			Metadata:     meta,
			ReceiptEmail: optionalString(stringExtra(req.Extra, ExtraReceiptEmailKey)),
		},
		LineItems: []*official.CheckoutSessionCreateLineItemParams{{
			Quantity: &quantity,
			PriceData: &official.CheckoutSessionCreateLineItemPriceDataParams{
				Currency:   &currency,
				UnitAmount: &req.Pricing.PayAmount.Amount,
				ProductData: &official.CheckoutSessionCreateLineItemPriceDataProductDataParams{
					Name:        stringPtr(firstNonEmpty(req.Subject, req.Body, req.OutTradeNo, req.PaymentID, "payment")),
					Description: optionalString(req.Body),
					Metadata:    meta,
				},
			},
		}},
	}
	if req.ExpireAt != nil {
		expiresAt := req.ExpireAt.Unix()
		params.ExpiresAt = &expiresAt
	}
	if methods := stringSliceExtra(req.Extra, ExtraPaymentMethodTypesKey); len(methods) > 0 {
		params.PaymentMethodTypes = stringPtrs(methods)
	}

	c.debug(ctx, createDebugEvent(debuglog.StageRequest, "create_checkout_session", req, "", params, nil, nil))
	session, err := c.checkoutService.CreateCheckoutSession(ctx, params)
	if err != nil {
		c.debug(ctx, createDebugEvent(debuglog.StageError, "create_checkout_session", req, "", params, nil, err))
		return nil, err
	}
	if session == nil || strings.TrimSpace(session.URL) == "" {
		err := fmt.Errorf("%w: stripe checkout url is required", payment.ErrPaymentActionRequired)
		c.debug(ctx, createDebugEvent(debuglog.StageError, "create_checkout_session", req, "", params, session, err))
		return nil, err
	}
	c.debug(ctx, createDebugEvent(debuglog.StageResponse, "create_checkout_session", req, session.ID, nil, session, nil))
	extra := map[string]any{ExtraCheckoutSessionIDKey: session.ID}
	providerTradeID := session.ID
	if session.PaymentIntent != nil && strings.TrimSpace(session.PaymentIntent.ID) != "" {
		extra[ExtraPaymentIntentIDKey] = session.PaymentIntent.ID
	}
	return &payment.CreatePaymentResponse{
		Provider:        payment.ProviderStripe,
		Channel:         payment.ChannelStripeCheckout,
		MerchantKey:     req.MerchantKey,
		PaymentID:       req.PaymentID,
		OrderID:         req.OrderID,
		OutTradeNo:      req.OutTradeNo,
		ProviderTradeID: providerTradeID,
		Status:          checkoutPaymentStatus(session),
		Pricing:         req.Pricing,
		Action: payment.PaymentAction{
			Type: payment.ActionRedirectURL,
			URL:  session.URL,
		},
		Raw:   session,
		Extra: extra,
	}, nil
}

func (c *Client) createPaymentIntent(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if c.paymentIntentService == nil {
		return nil, fmt.Errorf("%w: stripe payment intent service is required", payment.ErrAdapterNotFound)
	}
	currency := stripeCurrency(req.Pricing.PayAmount.Currency)
	enabled := true
	description := description(req)
	params := &official.PaymentIntentCreateParams{
		Amount:      &req.Pricing.PayAmount.Amount,
		Currency:    &currency,
		Description: optionalString(description),
		Metadata:    metadata(req),
		ReturnURL:   optionalString(req.ReturnURL),
		Customer:    optionalString(stringExtra(req.Extra, ExtraCustomerIDKey)),
		ReceiptEmail: optionalString(
			stringExtra(req.Extra, ExtraReceiptEmailKey),
		),
	}
	if methods := stringSliceExtra(req.Extra, ExtraPaymentMethodTypesKey); len(methods) > 0 {
		params.PaymentMethodTypes = stringPtrs(methods)
	} else {
		params.AutomaticPaymentMethods = &official.PaymentIntentCreateAutomaticPaymentMethodsParams{
			Enabled: &enabled,
		}
	}

	c.debug(ctx, createDebugEvent(debuglog.StageRequest, "create_payment_intent", req, "", params, nil, nil))
	intent, err := c.paymentIntentService.CreatePaymentIntent(ctx, params)
	if err != nil {
		c.debug(ctx, createDebugEvent(debuglog.StageError, "create_payment_intent", req, "", params, nil, err))
		return nil, err
	}
	if intent == nil || strings.TrimSpace(intent.ClientSecret) == "" {
		err := fmt.Errorf("%w: stripe payment intent client secret is required", payment.ErrPaymentActionRequired)
		c.debug(ctx, createDebugEvent(debuglog.StageError, "create_payment_intent", req, "", params, intent, err))
		return nil, err
	}
	c.debug(ctx, createDebugEvent(debuglog.StageResponse, "create_payment_intent", req, intent.ID, nil, intent, nil))
	return &payment.CreatePaymentResponse{
		Provider:        payment.ProviderStripe,
		Channel:         payment.ChannelStripeIntent,
		MerchantKey:     req.MerchantKey,
		PaymentID:       req.PaymentID,
		OrderID:         req.OrderID,
		OutTradeNo:      req.OutTradeNo,
		ProviderTradeID: intent.ID,
		Status:          intentPaymentStatus(intent),
		Pricing:         pricingFromIntent(req.Pricing, intent),
		Action: payment.PaymentAction{
			Type:  payment.ActionClientToken,
			Token: intent.ClientSecret,
			Params: map[string]any{
				ExtraPaymentIntentIDKey: intent.ID,
			},
		},
		Raw: intent,
		Extra: map[string]any{
			ExtraPaymentIntentIDKey: intent.ID,
		},
	}, nil
}

func servicesMissing(cfg Config) bool {
	return cfg.CheckoutService == nil || cfg.PaymentIntentService == nil || cfg.RefundService == nil
}
