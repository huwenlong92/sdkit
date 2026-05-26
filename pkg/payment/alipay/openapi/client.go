package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/core/payment"
)

func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.AppID) == "" {
		return nil, fmt.Errorf("%w: alipay app_id is required", payment.ErrInvalidRequest)
	}
	if cfg.Signer == nil {
		return nil, fmt.Errorf("%w: alipay signer is required", payment.ErrInvalidRequest)
	}
	gatewayURL := strings.TrimSpace(cfg.GatewayURL)
	if gatewayURL == "" {
		gatewayURL = DefaultGatewayURL
	}
	if _, err := url.ParseRequestURI(gatewayURL); err != nil {
		return nil, fmt.Errorf("%w: invalid alipay gateway url: %v", payment.ErrInvalidRequest, err)
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		appID:      strings.TrimSpace(cfg.AppID),
		gatewayURL: gatewayURL,
		notifyURL:  strings.TrimSpace(cfg.NotifyURL),
		returnURL:  strings.TrimSpace(cfg.ReturnURL),
		signer:     cfg.Signer,
		httpClient: httpClient,
		clock:      clock,
	}, nil
}

func (c *Client) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	method, productCode, err := methodForChannel(req.Channel)
	if err != nil {
		return nil, err
	}
	fields, err := c.buildCreateFields(ctx, req, method, productCode)
	if err != nil {
		return nil, err
	}
	action, err := actionForChannel(req.Channel, c.gatewayURL, fields)
	if err != nil {
		return nil, err
	}
	return &payment.CreatePaymentResponse{
		Provider:    payment.ProviderAlipay,
		Channel:     req.Channel,
		MerchantKey: req.MerchantKey,
		PaymentID:   req.PaymentID,
		OrderID:     req.OrderID,
		OutTradeNo:  req.OutTradeNo,
		Status:      payment.PaymentPending,
		Pricing:     req.Pricing,
		Action:      action,
	}, nil
}

func (c *Client) QueryPayment(ctx context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	outTradeNo := strings.TrimSpace(req.OutTradeNo)
	tradeNo := strings.TrimSpace(req.ProviderTradeID)
	if outTradeNo == "" && tradeNo == "" {
		return nil, fmt.Errorf("%w: alipay out_trade_no or trade_no is required", payment.ErrInvalidRequest)
	}
	bizRaw, err := json.Marshal(queryBizContent{
		OutTradeNo: outTradeNo,
		TradeNo:    tradeNo,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal alipay query biz content: %v", payment.ErrInvalidRequest, err)
	}
	fields, err := c.buildSignedFields(ctx, "alipay.trade.query", string(bizRaw), "", "")
	if err != nil {
		return nil, err
	}
	raw, err := c.postForm(ctx, fields)
	if err != nil {
		return nil, err
	}
	var gatewayResp queryGatewayResponse
	if err := json.Unmarshal(raw, &gatewayResp); err != nil {
		return nil, fmt.Errorf("%w: decode alipay query response: %v", payment.ErrInvalidRequest, err)
	}
	if gatewayResp.Response.Code != "10000" {
		return nil, fmt.Errorf("%w: alipay query failed %s %s", payment.ErrInvalidRequest, gatewayResp.Response.SubCode, gatewayResp.Response.SubMsg)
	}
	return queryPaymentResponse(req, gatewayResp.Response, raw), nil
}

func (c *Client) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	outTradeNo := firstNonEmpty(req.OutTradeNo, stringExtra(req.Extra, "out_trade_no"), req.PaymentID, req.OrderID)
	tradeNo := firstNonEmpty(req.ProviderTradeID, stringExtra(req.Extra, "trade_no"))
	if outTradeNo == "" && tradeNo == "" {
		return nil, fmt.Errorf("%w: alipay out_trade_no or trade_no is required", payment.ErrInvalidRequest)
	}
	if req.Amount.Refund.Amount <= 0 {
		return nil, fmt.Errorf("%w: alipay refund amount must be positive", payment.ErrInvalidAmount)
	}
	currency := payment.NormalizeCurrency(req.Amount.Refund.Currency)
	if currency != payment.DefaultCurrency {
		return nil, fmt.Errorf("%w: alipay refund currency %s", payment.ErrUnsupportedCapability, currency)
	}
	outRequestNo := firstNonEmpty(req.OutRefundNo, req.RefundID, stringExtra(req.Extra, "out_request_no"))
	bizRaw, err := json.Marshal(refundBizContent{
		OutTradeNo:   outTradeNo,
		TradeNo:      tradeNo,
		RefundAmount: amountString(req.Amount.Refund.Amount),
		RefundReason: strings.TrimSpace(req.Reason),
		OutRequestNo: outRequestNo,
		OperatorID:   stringExtra(req.Extra, "operator_id"),
		StoreID:      stringExtra(req.Extra, "store_id"),
		TerminalID:   stringExtra(req.Extra, "terminal_id"),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal alipay refund biz content: %v", payment.ErrInvalidRequest, err)
	}
	fields, err := c.buildSignedFields(ctx, "alipay.trade.refund", string(bizRaw), "", "")
	if err != nil {
		return nil, err
	}
	raw, err := c.postForm(ctx, fields)
	if err != nil {
		return nil, err
	}
	var gatewayResp refundGatewayResponse
	if err := json.Unmarshal(raw, &gatewayResp); err != nil {
		return nil, fmt.Errorf("%w: decode alipay refund response: %v", payment.ErrInvalidRequest, err)
	}
	if gatewayResp.Response.Code != "10000" {
		return nil, fmt.Errorf("%w: alipay refund failed %s %s", payment.ErrInvalidRequest, gatewayResp.Response.SubCode, gatewayResp.Response.SubMsg)
	}
	return refundPaymentResponse(req, gatewayResp.Response, outRequestNo, raw), nil
}

func (c *Client) QueryRefund(ctx context.Context, req payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	outRequestNo := firstNonEmpty(req.OutRefundNo, req.RefundID, stringExtra(req.Extra, "out_request_no"))
	if outRequestNo == "" {
		return nil, fmt.Errorf("%w: alipay out_request_no is required", payment.ErrInvalidRequest)
	}
	outTradeNo := firstNonEmpty(req.OutTradeNo, stringExtra(req.Extra, "out_trade_no"), req.PaymentID)
	tradeNo := firstNonEmpty(req.ProviderTradeID, stringExtra(req.Extra, "trade_no"))
	if outTradeNo == "" && tradeNo == "" {
		return nil, fmt.Errorf("%w: alipay out_trade_no or trade_no is required", payment.ErrInvalidRequest)
	}
	bizRaw, err := json.Marshal(refundQueryBizContent{
		OutTradeNo:   outTradeNo,
		TradeNo:      tradeNo,
		OutRequestNo: outRequestNo,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal alipay refund query biz content: %v", payment.ErrInvalidRequest, err)
	}
	fields, err := c.buildSignedFields(ctx, "alipay.trade.fastpay.refund.query", string(bizRaw), "", "")
	if err != nil {
		return nil, err
	}
	raw, err := c.postForm(ctx, fields)
	if err != nil {
		return nil, err
	}
	var gatewayResp refundQueryGatewayResponse
	if err := json.Unmarshal(raw, &gatewayResp); err != nil {
		return nil, fmt.Errorf("%w: decode alipay refund query response: %v", payment.ErrInvalidRequest, err)
	}
	if gatewayResp.Response.Code != "10000" {
		return nil, fmt.Errorf("%w: alipay refund query failed %s %s", payment.ErrInvalidRequest, gatewayResp.Response.SubCode, gatewayResp.Response.SubMsg)
	}
	return refundQueryPaymentResponse(req, gatewayResp.Response, raw), nil
}

func (c *Client) buildCreateFields(ctx context.Context, req payment.CreatePaymentRequest, method, productCode string) (map[string]string, error) {
	outTradeNo := firstNonEmpty(req.OutTradeNo, req.PaymentID)
	if outTradeNo == "" {
		return nil, fmt.Errorf("%w: alipay out_trade_no is required", payment.ErrInvalidRequest)
	}
	subject := firstNonEmpty(req.Subject, req.Body, req.OutTradeNo, req.PaymentID)
	if subject == "" {
		return nil, fmt.Errorf("%w: alipay subject is required", payment.ErrInvalidRequest)
	}
	if req.Pricing.PayAmount.Amount <= 0 {
		return nil, fmt.Errorf("%w: alipay amount must be positive", payment.ErrInvalidAmount)
	}
	currency := payment.NormalizeCurrency(req.Pricing.PayAmount.Currency)
	if currency != payment.DefaultCurrency {
		return nil, fmt.Errorf("%w: alipay currency %s", payment.ErrUnsupportedCapability, currency)
	}

	biz := bizContent{
		OutTradeNo:     outTradeNo,
		TotalAmount:    amountString(req.Pricing.PayAmount.Amount),
		Subject:        subject,
		Body:           strings.TrimSpace(req.Body),
		ProductCode:    productCode,
		TimeoutExpress: timeoutExpress(req.ExpireAt, c.clock()),
	}
	bizRaw, err := json.Marshal(biz)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal alipay biz content: %v", payment.ErrInvalidRequest, err)
	}

	return c.buildSignedFields(ctx, method, string(bizRaw), firstNonEmpty(req.NotifyURL, c.notifyURL), firstNonEmpty(req.ReturnURL, c.returnURL))
}

func (c *Client) buildSignedFields(ctx context.Context, method, bizContent, notifyURL, returnURL string) (map[string]string, error) {
	fields := map[string]string{
		"app_id":      c.appID,
		"method":      method,
		"format":      "JSON",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"timestamp":   c.clock().Format("2006-01-02 15:04:05"),
		"version":     "1.0",
		"biz_content": bizContent,
	}
	if notifyURL != "" {
		fields["notify_url"] = notifyURL
	}
	if returnURL != "" {
		fields["return_url"] = returnURL
	}
	signature, err := c.signer.Sign(ctx, signContent(fields))
	if err != nil {
		return nil, err
	}
	fields["sign"] = signature
	return fields, nil
}

func (c *Client) postForm(ctx context.Context, fields map[string]string) ([]byte, error) {
	values := make(url.Values, len(fields))
	for key, value := range fields {
		values.Set(key, value)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.gatewayURL, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: build alipay request: %v", payment.ErrInvalidRequest, err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: alipay http status %d: %s", payment.ErrInvalidRequest, resp.StatusCode, string(body))
	}
	return body, nil
}

func methodForChannel(channel payment.Channel) (method string, productCode string, err error) {
	switch channel {
	case payment.ChannelAlipayApp:
		return "alipay.trade.app.pay", "QUICK_MSECURITY_PAY", nil
	case payment.ChannelAlipayWap:
		return "alipay.trade.wap.pay", "QUICK_WAP_WAY", nil
	case payment.ChannelAlipayPage:
		return "alipay.trade.page.pay", "FAST_INSTANT_TRADE_PAY", nil
	default:
		return "", "", fmt.Errorf("%w: alipay channel %s", payment.ErrUnsupportedChannel, channel)
	}
}

func actionForChannel(channel payment.Channel, gatewayURL string, fields map[string]string) (payment.PaymentAction, error) {
	switch channel {
	case payment.ChannelAlipayApp:
		return payment.PaymentAction{
			Type: payment.ActionSDKParams,
			Params: map[string]any{
				"order_string": encodedFields(fields),
				"fields":       cloneFields(fields),
			},
		}, nil
	case payment.ChannelAlipayWap, payment.ChannelAlipayPage:
		return payment.PaymentAction{
			Type:   payment.ActionHTMLForm,
			Method: "POST",
			URL:    gatewayURL,
			Fields: cloneFields(fields),
		}, nil
	default:
		return payment.PaymentAction{}, fmt.Errorf("%w: alipay channel %s", payment.ErrUnsupportedChannel, channel)
	}
}

func signContent(fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for key, value := range fields {
		if key == "sign" || strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+fields[key])
	}
	return strings.Join(parts, "&")
}

func encodedFields(fields map[string]string) string {
	values := make(url.Values, len(fields))
	for key, value := range fields {
		values.Set(key, value)
	}
	return values.Encode()
}

func cloneFields(fields map[string]string) map[string]string {
	out := make(map[string]string, len(fields))
	for key, value := range fields {
		out[key] = value
	}
	return out
}

func amountString(minor int64) string {
	major := minor / 100
	fraction := minor % 100
	return fmt.Sprintf("%d.%02d", major, fraction)
}

func timeoutExpress(expireAt *time.Time, now time.Time) string {
	if expireAt == nil || !expireAt.After(now) {
		return ""
	}
	minutes := int(expireAt.Sub(now).Minutes())
	if minutes <= 0 {
		minutes = 1
	}
	return fmt.Sprintf("%dm", minutes)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func stringExtra(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	value, ok := extra[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func queryPaymentResponse(req payment.QueryPaymentRequest, resp queryResponse, raw []byte) *payment.QueryPaymentResponse {
	status := alipayTradeStatus(resp.TradeStatus)
	amount := decimalToMinor(resp.TotalAmount)
	var paidAt *time.Time
	if resp.SendPayDate != "" {
		if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", resp.SendPayDate, time.Local); err == nil {
			paidAt = &parsed
		}
	}
	return &payment.QueryPaymentResponse{
		Provider:        payment.ProviderAlipay,
		Channel:         req.Channel,
		MerchantKey:     req.MerchantKey,
		PaymentID:       req.PaymentID,
		OrderID:         req.OrderID,
		OutTradeNo:      firstNonEmpty(resp.OutTradeNo, req.OutTradeNo),
		ProviderTradeID: firstNonEmpty(resp.TradeNo, req.ProviderTradeID),
		Status:          status,
		Pricing:         payment.CNY(amount),
		PaidAt:          paidAt,
		Raw:             string(raw),
		Extra: map[string]any{
			"alipay_code":         resp.Code,
			"alipay_msg":          resp.Msg,
			"alipay_trade_status": resp.TradeStatus,
		},
	}
}

func refundPaymentResponse(req payment.RefundRequest, resp refundResponse, outRequestNo string, raw []byte) *payment.RefundResponse {
	refundAmount := decimalToMinor(resp.RefundFee)
	if refundAmount == 0 {
		refundAmount = req.Amount.Refund.Amount
	}
	return &payment.RefundResponse{
		Provider:        payment.ProviderAlipay,
		Channel:         req.Channel,
		MerchantKey:     req.MerchantKey,
		PaymentID:       req.PaymentID,
		OrderID:         req.OrderID,
		OutTradeNo:      firstNonEmpty(resp.OutTradeNo, req.OutTradeNo),
		ProviderTradeID: firstNonEmpty(resp.TradeNo, req.ProviderTradeID),
		RefundID:        req.RefundID,
		OutRefundNo:     firstNonEmpty(outRequestNo, req.OutRefundNo),
		Status:          payment.RefundSucceeded,
		Amount: payment.RefundAmount{
			Refund: payment.Money{Amount: refundAmount, Currency: payment.DefaultCurrency},
			Settle: payment.Money{Amount: refundAmount, Currency: payment.DefaultCurrency},
		},
		Raw: string(raw),
		Extra: map[string]any{
			"alipay_code":        resp.Code,
			"alipay_msg":         resp.Msg,
			"alipay_fund_change": resp.FundChange,
		},
	}
}

func refundQueryPaymentResponse(req payment.QueryRefundRequest, resp refundQueryResponse, raw []byte) *payment.QueryRefundResponse {
	refundAmount := decimalToMinor(resp.RefundAmount)
	return &payment.QueryRefundResponse{
		Provider:         payment.ProviderAlipay,
		Channel:          req.Channel,
		MerchantKey:      req.MerchantKey,
		PaymentID:        req.PaymentID,
		OutTradeNo:       firstNonEmpty(resp.OutTradeNo, req.OutTradeNo),
		ProviderTradeID:  firstNonEmpty(resp.TradeNo, req.ProviderTradeID),
		RefundID:         req.RefundID,
		OutRefundNo:      firstNonEmpty(resp.OutRequestNo, req.OutRefundNo),
		ProviderRefundID: req.ProviderRefundID,
		Status:           refundStatus(resp.RefundStatus),
		Amount: payment.RefundAmount{
			Refund: payment.Money{Amount: refundAmount, Currency: payment.DefaultCurrency},
			Settle: payment.Money{Amount: refundAmount, Currency: payment.DefaultCurrency},
		},
		Raw: string(raw),
		Extra: map[string]any{
			"alipay_code":          resp.Code,
			"alipay_msg":           resp.Msg,
			"alipay_out_trade_no":  resp.OutTradeNo,
			"alipay_refund_status": resp.RefundStatus,
		},
	}
}

func refundStatus(status string) payment.RefundStatus {
	switch status {
	case "", "REFUND_SUCCESS":
		return payment.RefundSucceeded
	default:
		return payment.RefundProcessing
	}
}

func alipayTradeStatus(status string) payment.PaymentStatus {
	switch status {
	case "WAIT_BUYER_PAY":
		return payment.PaymentPending
	case "TRADE_SUCCESS", "TRADE_FINISHED":
		return payment.PaymentSucceeded
	case "TRADE_CLOSED":
		return payment.PaymentClosed
	default:
		return payment.PaymentProcessing
	}
}

func decimalToMinor(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	parts := strings.SplitN(value, ".", 2)
	major, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0
	}
	fraction := "00"
	if len(parts) == 2 {
		fraction = parts[1] + "00"
	}
	minor, err := strconv.ParseInt(fraction[:2], 10, 64)
	if err != nil {
		return 0
	}
	return major*100 + minor
}
