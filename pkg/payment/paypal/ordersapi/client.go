//go:build sdkit_payment_paypal

package ordersapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/core/payment"
)

func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.ClientID) == "" {
		return nil, fmt.Errorf("%w: paypal client_id is required", payment.ErrInvalidRequest)
	}
	if strings.TrimSpace(cfg.ClientSecret) == "" {
		return nil, fmt.Errorf("%w: paypal client_secret is required", payment.ErrInvalidRequest)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultSandboxBaseURL
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("%w: invalid paypal base url: %v", payment.ErrInvalidRequest, err)
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Client{
		clientID:         strings.TrimSpace(cfg.ClientID),
		clientSecret:     strings.TrimSpace(cfg.ClientSecret),
		baseURL:          baseURL,
		returnURL:        strings.TrimSpace(cfg.ReturnURL),
		cancelURL:        strings.TrimSpace(cfg.CancelURL),
		httpClient:       httpClient,
		clock:            clock,
		debugLogger:      cfg.DebugLogger,
		debugPayloadMode: cfg.DebugPayloadMode,
	}, nil
}

func (c *Client) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	if req.Channel != payment.ChannelPayPalOrder {
		return nil, fmt.Errorf("%w: paypal channel %s", payment.ErrUnsupportedChannel, req.Channel)
	}
	returnURL := firstNonEmpty(req.ReturnURL, c.returnURL)
	if returnURL == "" {
		return nil, fmt.Errorf("%w: paypal return url is required", payment.ErrInvalidRequest)
	}
	cancelURL := firstNonEmpty(stringExtra(req.Extra, ExtraCancelURLKey), c.cancelURL, returnURL)
	amount, err := amountFromMoney(req.Pricing.PayAmount)
	if err != nil {
		return nil, err
	}
	body := createOrderRequest{
		Intent: "CAPTURE",
		PurchaseUnits: []purchaseUnit{{
			ReferenceID: firstNonEmpty(req.OutTradeNo, req.PaymentID, req.OrderID),
			Description: description(req),
			CustomID:    firstNonEmpty(req.PaymentID, req.OrderID),
			InvoiceID:   optionalInvoiceID(req.OutTradeNo),
			Amount:      amount,
		}},
		PaymentSource: &paymentSourcePayload{PayPal: paypalPaymentSource{
			ExperienceContext: paypalExperienceContext{
				ReturnURL:  returnURL,
				CancelURL:  cancelURL,
				UserAction: "PAY_NOW",
			},
		}},
	}
	var raw orderResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v2/checkout/orders", body, &raw); err != nil {
		return nil, err
	}
	approveURL := linkByRel(raw.Links, "approve")
	if raw.ID == "" || approveURL == "" {
		return nil, fmt.Errorf("%w: paypal approval url is required", payment.ErrPaymentActionRequired)
	}
	return &payment.CreatePaymentResponse{
		Provider:        payment.ProviderPayPal,
		Channel:         payment.ChannelPayPalOrder,
		MerchantKey:     req.MerchantKey,
		PaymentID:       req.PaymentID,
		OrderID:         req.OrderID,
		OutTradeNo:      req.OutTradeNo,
		ProviderTradeID: raw.ID,
		Status:          orderStatus(raw.Status),
		Pricing:         req.Pricing,
		Action: payment.PaymentAction{
			Type: payment.ActionRedirectURL,
			URL:  approveURL,
		},
		Raw: raw,
		Extra: map[string]any{
			ExtraOrderIDKey: raw.ID,
		},
	}, nil
}

func (c *Client) QueryPayment(ctx context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	orderID := firstNonEmpty(req.ProviderTradeID, stringExtra(req.Extra, ExtraOrderIDKey))
	if orderID == "" {
		return nil, fmt.Errorf("%w: paypal order id is required", payment.ErrPaymentReference)
	}
	var raw orderResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v2/checkout/orders/"+url.PathEscape(orderID), nil, &raw); err != nil {
		return nil, err
	}
	pricing := payment.PaymentPricing{}
	if len(raw.PurchaseUnits) > 0 {
		amount, err := moneyFromAmount(raw.PurchaseUnits[0].Amount)
		if err == nil {
			pricing.PayAmount = amount
		}
	}
	return &payment.QueryPaymentResponse{
		Provider:        payment.ProviderPayPal,
		Channel:         payment.ChannelPayPalOrder,
		MerchantKey:     req.MerchantKey,
		PaymentID:       req.PaymentID,
		OrderID:         req.OrderID,
		OutTradeNo:      req.OutTradeNo,
		ProviderTradeID: raw.ID,
		Status:          orderStatus(raw.Status),
		Pricing:         pricing,
		Raw:             raw,
		Extra: map[string]any{
			ExtraOrderIDKey: raw.ID,
		},
	}, nil
}

func (c *Client) CapturePayment(ctx context.Context, req CapturePaymentRequest) (*payment.QueryPaymentResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	orderID := firstNonEmpty(req.ProviderTradeID, stringExtra(req.Extra, ExtraOrderIDKey))
	if orderID == "" {
		return nil, fmt.Errorf("%w: paypal order id is required", payment.ErrPaymentReference)
	}
	var raw captureOrderResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v2/checkout/orders/"+url.PathEscape(orderID)+"/capture", map[string]any{}, &raw); err != nil {
		return nil, err
	}
	captureID := firstCaptureID(raw)
	return &payment.QueryPaymentResponse{
		Provider:        payment.ProviderPayPal,
		Channel:         payment.ChannelPayPalOrder,
		MerchantKey:     req.MerchantKey,
		PaymentID:       req.PaymentID,
		OrderID:         req.OrderID,
		OutTradeNo:      req.OutTradeNo,
		ProviderTradeID: raw.ID,
		Status:          orderStatus(raw.Status),
		Raw:             raw,
		Extra: map[string]any{
			ExtraOrderIDKey:   raw.ID,
			ExtraCaptureIDKey: captureID,
		},
	}, nil
}

func (c *Client) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	captureID := firstNonEmpty(stringExtra(req.Extra, ExtraCaptureIDKey), req.ProviderTradeID)
	if captureID == "" {
		return nil, fmt.Errorf("%w: paypal capture id is required", payment.ErrPaymentReference)
	}
	amount, err := amountFromMoney(req.Amount.Refund)
	if err != nil {
		return nil, err
	}
	body := refundRequest{Amount: amount, Note: strings.TrimSpace(req.Reason)}
	var raw refundResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v2/payments/captures/"+url.PathEscape(captureID)+"/refund", body, &raw); err != nil {
		return nil, err
	}
	return &payment.RefundResponse{
		Provider:         payment.ProviderPayPal,
		Channel:          payment.ChannelPayPalOrder,
		MerchantKey:      req.MerchantKey,
		PaymentID:        req.PaymentID,
		OrderID:          req.OrderID,
		OutTradeNo:       req.OutTradeNo,
		ProviderTradeID:  req.ProviderTradeID,
		RefundID:         req.RefundID,
		OutRefundNo:      req.OutRefundNo,
		ProviderRefundID: raw.ID,
		Status:           refundStatus(raw.Status),
		Amount:           payment.RefundAmount{Refund: mustMoneyFromAmount(raw.Amount, req.Amount.Refund)},
		Raw:              raw,
	}, nil
}

func (c *Client) QueryRefund(ctx context.Context, req payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	refundID := firstNonEmpty(req.ProviderRefundID, req.RefundID)
	if refundID == "" {
		return nil, fmt.Errorf("%w: paypal refund id is required", payment.ErrPaymentReference)
	}
	var raw refundResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v2/payments/refunds/"+url.PathEscape(refundID), nil, &raw); err != nil {
		return nil, err
	}
	return &payment.QueryRefundResponse{
		Provider:         payment.ProviderPayPal,
		Channel:          payment.ChannelPayPalOrder,
		MerchantKey:      req.MerchantKey,
		PaymentID:        req.PaymentID,
		OutTradeNo:       req.OutTradeNo,
		ProviderTradeID:  req.ProviderTradeID,
		RefundID:         req.RefundID,
		OutRefundNo:      req.OutRefundNo,
		ProviderRefundID: raw.ID,
		Status:           refundStatus(raw.Status),
		Amount:           payment.RefundAmount{Refund: mustMoneyFromAmount(raw.Amount, payment.Money{})},
		Raw:              raw,
	}, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%w: marshal paypal request: %v", payment.ErrInvalidRequest, err)
		}
		reader = bytes.NewReader(raw)
	}
	c.debug(ctx, debugEvent{
		Stage:     "request",
		Operation: method + " " + path,
		Request:   body,
	})
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("%w: build paypal request: %v", payment.ErrInvalidRequest, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.debug(ctx, debugEvent{Stage: "error", Operation: method + " " + path, Request: body, Err: err})
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		err := fmt.Errorf("%w: paypal %s %s failed: status %d body %s", payment.ErrInvalidRequest, method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
		c.debug(ctx, debugEvent{Stage: "error", Operation: method + " " + path, Request: body, Response: string(raw), Err: err})
		return err
	}
	if out == nil || len(raw) == 0 {
		c.debug(ctx, debugEvent{Stage: "response", Operation: method + " " + path, Request: body, Response: nil})
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		err = fmt.Errorf("%w: decode paypal response: %v", payment.ErrInvalidRequest, err)
		c.debug(ctx, debugEvent{Stage: "error", Operation: method + " " + path, Request: body, Response: string(raw), Err: err})
		return err
	}
	c.debug(ctx, debugEvent{Stage: "response", Operation: method + " " + path, Request: body, Response: out})
	return nil
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token.AccessToken != "" && c.clock().Before(c.token.expiresAt.Add(-30*time.Second)) {
		return c.token.AccessToken, nil
	}
	values := url.Values{}
	values.Set("grant_type", "client_credentials")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/oauth2/token", strings.NewReader(values.Encode()))
	if err != nil {
		return "", fmt.Errorf("%w: build paypal token request: %v", payment.ErrInvalidRequest, err)
	}
	req.SetBasicAuth(c.clientID, c.clientSecret)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("%w: paypal oauth token failed: status %d body %s", payment.ErrInvalidRequest, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var token oauthToken
	if err := json.Unmarshal(raw, &token); err != nil {
		return "", fmt.Errorf("%w: decode paypal oauth token: %v", payment.ErrInvalidRequest, err)
	}
	if token.AccessToken == "" {
		return "", fmt.Errorf("%w: paypal access token is empty", payment.ErrInvalidRequest)
	}
	if token.ExpiresIn <= 0 {
		token.ExpiresIn = 300
	}
	token.expiresAt = c.clock().Add(time.Duration(token.ExpiresIn) * time.Second)
	c.token = token
	return token.AccessToken, nil
}
