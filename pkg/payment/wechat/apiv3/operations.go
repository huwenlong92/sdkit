//go:build sdkit_payment_wechat

package apiv3

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/huwenlong92/sdkit/core/payment"
	wechatpayments "github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/app"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/h5"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/jsapi"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/services/refunddomestic"
)

func (c *Client) QueryPayment(ctx context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	transactionID := firstNonEmpty(req.ProviderTradeID, stringExtra(req.Extra, "transaction_id"))
	outTradeNo := firstNonEmpty(req.OutTradeNo, req.PaymentID)
	if transactionID == "" && outTradeNo == "" {
		return nil, fmt.Errorf("%w: wechat transaction_id or out_trade_no is required", payment.ErrInvalidRequest)
	}
	transaction, err := c.queryPayment(ctx, req.Channel, transactionID, outTradeNo)
	if err != nil {
		return nil, err
	}
	return queryPaymentResponse(req, transaction), nil
}

func (c *Client) ClosePayment(ctx context.Context, req payment.ClosePaymentRequest) error {
	if ctx == nil {
		return payment.ErrNilContext
	}
	outTradeNo := firstNonEmpty(req.OutTradeNo, req.PaymentID)
	if outTradeNo == "" {
		return fmt.Errorf("%w: wechat out_trade_no is required", payment.ErrInvalidRequest)
	}
	return c.closePayment(ctx, req.Channel, outTradeNo)
}

func (c *Client) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	if c.refundService == nil {
		return nil, fmt.Errorf("%w: wechat refund service is required", payment.ErrAdapterNotFound)
	}
	outTradeNo := firstNonEmpty(req.OutTradeNo, req.PaymentID, req.OrderID)
	transactionID := firstNonEmpty(req.ProviderTradeID, stringExtra(req.Extra, "transaction_id"))
	if outTradeNo == "" && transactionID == "" {
		return nil, fmt.Errorf("%w: wechat transaction_id or out_trade_no is required", payment.ErrInvalidRequest)
	}
	outRefundNo := firstNonEmpty(req.OutRefundNo, req.RefundID)
	if outRefundNo == "" {
		return nil, fmt.Errorf("%w: wechat out_refund_no is required", payment.ErrInvalidRequest)
	}
	total, ok := int64Extra(req.Extra, ExtraRefundTotalAmountKey)
	if !ok || total <= 0 {
		return nil, fmt.Errorf("%w: wechat refund total_amount is required", payment.ErrInvalidRequest)
	}
	if req.Amount.Refund.Amount <= 0 {
		return nil, fmt.Errorf("%w: wechat refund amount must be positive", payment.ErrInvalidAmount)
	}
	currency := payment.NormalizeCurrency(req.Amount.Refund.Currency)
	if currency != payment.DefaultCurrency {
		return nil, fmt.Errorf("%w: wechat refund currency %s", payment.ErrUnsupportedCapability, currency)
	}
	officialReq := refunddomestic.CreateRequest{
		TransactionId: optionalString(transactionID),
		OutTradeNo:    optionalString(outTradeNo),
		OutRefundNo:   stringPtr(outRefundNo),
		Reason:        optionalString(req.Reason),
		NotifyUrl:     optionalString(stringExtra(req.Extra, ExtraRefundNotifyURLKey)),
		Amount: &refunddomestic.AmountReq{
			Refund:   &req.Amount.Refund.Amount,
			Total:    &total,
			Currency: stringPtr(currency),
		},
	}
	resp, _, err := c.refundService.Create(ctx, officialReq)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("%w: nil wechat refund response", payment.ErrInvalidRequest)
	}
	return refundResponse(req, resp), nil
}

func (c *Client) QueryRefund(ctx context.Context, req payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	if c.refundService == nil {
		return nil, fmt.Errorf("%w: wechat refund service is required", payment.ErrAdapterNotFound)
	}
	outRefundNo := firstNonEmpty(req.OutRefundNo, req.RefundID)
	if outRefundNo == "" {
		return nil, fmt.Errorf("%w: wechat out_refund_no is required", payment.ErrInvalidRequest)
	}
	resp, _, err := c.refundService.QueryByOutRefundNo(ctx, refunddomestic.QueryByOutRefundNoRequest{OutRefundNo: stringPtr(outRefundNo)})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("%w: nil wechat refund query response", payment.ErrInvalidRequest)
	}
	return queryRefundResponse(req, resp), nil
}

func (c *Client) queryPayment(ctx context.Context, channel payment.Channel, transactionID, outTradeNo string) (*wechatpayments.Transaction, error) {
	switch channel {
	case payment.ChannelWechatApp:
		if c.appService == nil {
			return nil, fmt.Errorf("%w: wechat app service is required", payment.ErrAdapterNotFound)
		}
		if transactionID != "" {
			resp, _, err := c.appService.QueryOrderById(ctx, app.QueryOrderByIdRequest{TransactionId: stringPtr(transactionID), Mchid: &c.merchantID})
			return resp, err
		}
		resp, _, err := c.appService.QueryOrderByOutTradeNo(ctx, app.QueryOrderByOutTradeNoRequest{OutTradeNo: stringPtr(outTradeNo), Mchid: &c.merchantID})
		return resp, err
	case payment.ChannelWechatMiniProgram:
		if c.jsapiService == nil {
			return nil, fmt.Errorf("%w: wechat jsapi service is required", payment.ErrAdapterNotFound)
		}
		if transactionID != "" {
			resp, _, err := c.jsapiService.QueryOrderById(ctx, jsapi.QueryOrderByIdRequest{TransactionId: stringPtr(transactionID), Mchid: &c.merchantID})
			return resp, err
		}
		resp, _, err := c.jsapiService.QueryOrderByOutTradeNo(ctx, jsapi.QueryOrderByOutTradeNoRequest{OutTradeNo: stringPtr(outTradeNo), Mchid: &c.merchantID})
		return resp, err
	case payment.ChannelWechatH5:
		if c.h5Service == nil {
			return nil, fmt.Errorf("%w: wechat h5 service is required", payment.ErrAdapterNotFound)
		}
		if transactionID != "" {
			resp, _, err := c.h5Service.QueryOrderById(ctx, h5.QueryOrderByIdRequest{TransactionId: stringPtr(transactionID), Mchid: &c.merchantID})
			return resp, err
		}
		resp, _, err := c.h5Service.QueryOrderByOutTradeNo(ctx, h5.QueryOrderByOutTradeNoRequest{OutTradeNo: stringPtr(outTradeNo), Mchid: &c.merchantID})
		return resp, err
	case payment.ChannelWechatNative:
		if c.nativeService == nil {
			return nil, fmt.Errorf("%w: wechat native service is required", payment.ErrAdapterNotFound)
		}
		if transactionID != "" {
			resp, _, err := c.nativeService.QueryOrderById(ctx, native.QueryOrderByIdRequest{TransactionId: stringPtr(transactionID), Mchid: &c.merchantID})
			return resp, err
		}
		resp, _, err := c.nativeService.QueryOrderByOutTradeNo(ctx, native.QueryOrderByOutTradeNoRequest{OutTradeNo: stringPtr(outTradeNo), Mchid: &c.merchantID})
		return resp, err
	default:
		return nil, fmt.Errorf("%w: wechat channel %s", payment.ErrUnsupportedChannel, channel)
	}
}

func (c *Client) closePayment(ctx context.Context, channel payment.Channel, outTradeNo string) error {
	switch channel {
	case payment.ChannelWechatApp:
		if c.appService == nil {
			return fmt.Errorf("%w: wechat app service is required", payment.ErrAdapterNotFound)
		}
		_, err := c.appService.CloseOrder(ctx, app.CloseOrderRequest{OutTradeNo: stringPtr(outTradeNo), Mchid: &c.merchantID})
		return err
	case payment.ChannelWechatMiniProgram:
		if c.jsapiService == nil {
			return fmt.Errorf("%w: wechat jsapi service is required", payment.ErrAdapterNotFound)
		}
		_, err := c.jsapiService.CloseOrder(ctx, jsapi.CloseOrderRequest{OutTradeNo: stringPtr(outTradeNo), Mchid: &c.merchantID})
		return err
	case payment.ChannelWechatH5:
		if c.h5Service == nil {
			return fmt.Errorf("%w: wechat h5 service is required", payment.ErrAdapterNotFound)
		}
		_, err := c.h5Service.CloseOrder(ctx, h5.CloseOrderRequest{OutTradeNo: stringPtr(outTradeNo), Mchid: &c.merchantID})
		return err
	case payment.ChannelWechatNative:
		if c.nativeService == nil {
			return fmt.Errorf("%w: wechat native service is required", payment.ErrAdapterNotFound)
		}
		_, err := c.nativeService.CloseOrder(ctx, native.CloseOrderRequest{OutTradeNo: stringPtr(outTradeNo), Mchid: &c.merchantID})
		return err
	default:
		return fmt.Errorf("%w: wechat channel %s", payment.ErrUnsupportedChannel, channel)
	}
}

func queryPaymentResponse(req payment.QueryPaymentRequest, transaction *wechatpayments.Transaction) *payment.QueryPaymentResponse {
	if transaction == nil {
		return &payment.QueryPaymentResponse{Provider: payment.ProviderWechat, Channel: req.Channel, Status: payment.PaymentProcessing}
	}
	amount := int64(0)
	currency := payment.DefaultCurrency
	if transaction.Amount != nil {
		amount = int64Value(transaction.Amount.Total)
		currency = firstNonEmpty(stringValue(transaction.Amount.Currency), payment.DefaultCurrency)
	}
	return &payment.QueryPaymentResponse{
		Provider:        payment.ProviderWechat,
		Channel:         req.Channel,
		MerchantKey:     req.MerchantKey,
		PaymentID:       req.PaymentID,
		OrderID:         req.OrderID,
		OutTradeNo:      firstNonEmpty(stringValue(transaction.OutTradeNo), req.OutTradeNo),
		ProviderTradeID: firstNonEmpty(stringValue(transaction.TransactionId), req.ProviderTradeID),
		Status:          wechatTradeStatus(stringValue(transaction.TradeState)),
		Pricing: payment.PaymentPricing{
			PayAmount: payment.Money{Amount: amount, Currency: currency},
		},
		PaidAt: parseWechatTime(stringValue(transaction.SuccessTime)),
		Raw:    transaction,
		Extra: map[string]any{
			"wechat_trade_state":      stringValue(transaction.TradeState),
			"wechat_trade_state_desc": stringValue(transaction.TradeStateDesc),
			"wechat_trade_type":       stringValue(transaction.TradeType),
		},
	}
}

func refundResponse(req payment.RefundRequest, refund *refunddomestic.Refund) *payment.RefundResponse {
	amount := refundAmount(refund)
	return &payment.RefundResponse{
		Provider:         payment.ProviderWechat,
		Channel:          req.Channel,
		MerchantKey:      req.MerchantKey,
		PaymentID:        req.PaymentID,
		OrderID:          req.OrderID,
		OutTradeNo:       firstNonEmpty(stringValue(refund.OutTradeNo), req.OutTradeNo),
		ProviderTradeID:  firstNonEmpty(stringValue(refund.TransactionId), req.ProviderTradeID),
		RefundID:         req.RefundID,
		OutRefundNo:      firstNonEmpty(stringValue(refund.OutRefundNo), req.OutRefundNo),
		ProviderRefundID: stringValue(refund.RefundId),
		Status:           wechatRefundStatus(refundStatusValue(refund.Status)),
		Amount:           amount,
		Raw:              refund,
		Extra: map[string]any{
			"wechat_refund_status": refundStatusValue(refund.Status),
		},
	}
}

func queryRefundResponse(req payment.QueryRefundRequest, refund *refunddomestic.Refund) *payment.QueryRefundResponse {
	amount := refundAmount(refund)
	return &payment.QueryRefundResponse{
		Provider:         payment.ProviderWechat,
		Channel:          req.Channel,
		MerchantKey:      req.MerchantKey,
		PaymentID:        req.PaymentID,
		OutTradeNo:       firstNonEmpty(stringValue(refund.OutTradeNo), req.OutTradeNo),
		ProviderTradeID:  firstNonEmpty(stringValue(refund.TransactionId), req.ProviderTradeID),
		RefundID:         req.RefundID,
		OutRefundNo:      firstNonEmpty(stringValue(refund.OutRefundNo), req.OutRefundNo),
		ProviderRefundID: stringValue(refund.RefundId),
		Status:           wechatRefundStatus(refundStatusValue(refund.Status)),
		Amount:           amount,
		Raw:              refund,
		Extra: map[string]any{
			"wechat_refund_status": refundStatusValue(refund.Status),
		},
	}
}

func refundAmount(refund *refunddomestic.Refund) payment.RefundAmount {
	if refund == nil || refund.Amount == nil {
		return payment.RefundAmount{}
	}
	currency := firstNonEmpty(stringValue(refund.Amount.Currency), payment.DefaultCurrency)
	refundValue := int64Value(refund.Amount.Refund)
	settleValue := int64Value(refund.Amount.SettlementRefund)
	if settleValue == 0 {
		settleValue = refundValue
	}
	return payment.RefundAmount{
		Refund: payment.Money{Amount: refundValue, Currency: currency},
		Settle: payment.Money{Amount: settleValue, Currency: currency},
	}
}

func wechatTradeStatus(status string) payment.PaymentStatus {
	switch status {
	case "SUCCESS":
		return payment.PaymentSucceeded
	case "REFUND":
		return payment.PaymentRefunding
	case "NOTPAY":
		return payment.PaymentPending
	case "CLOSED", "REVOKED":
		return payment.PaymentClosed
	case "USERPAYING":
		return payment.PaymentProcessing
	case "PAYERROR":
		return payment.PaymentFailed
	default:
		return payment.PaymentProcessing
	}
}

func wechatRefundStatus(status string) payment.RefundStatus {
	switch status {
	case "SUCCESS":
		return payment.RefundSucceeded
	case "CLOSED":
		return payment.RefundClosed
	case "PROCESSING":
		return payment.RefundProcessing
	case "ABNORMAL":
		return payment.RefundFailed
	default:
		return payment.RefundProcessing
	}
}

func refundStatusValue(status *refunddomestic.Status) string {
	if status == nil {
		return ""
	}
	return string(*status)
}

func parseWechatTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func int64Extra(extra map[string]any, key string) (int64, bool) {
	if extra == nil {
		return 0, false
	}
	value, ok := extra[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case int32:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case string:
		parsed, err := strconv.ParseInt(typed, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
