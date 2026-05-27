//go:build sdkit_payment_wechat

package apiv3

import (
	"context"
	"crypto/rsa"
	"net/http"

	wechatcore "github.com/wechatpay-apiv3/wechatpay-go/core"
	wechatpayments "github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/app"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/h5"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/jsapi"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/services/refunddomestic"
)

const (
	ExtraOpenIDKey            = "openid"
	ExtraClientIPKey          = "client_ip"
	ExtraRefundTotalAmountKey = "total_amount"
	ExtraRefundNotifyURLKey   = "refund_notify_url"
)

type AppService interface {
	PrepayWithRequestPayment(ctx context.Context, req app.PrepayRequest) (*app.PrepayWithRequestPaymentResponse, *wechatcore.APIResult, error)
	QueryOrderById(ctx context.Context, req app.QueryOrderByIdRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error)
	QueryOrderByOutTradeNo(ctx context.Context, req app.QueryOrderByOutTradeNoRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error)
	CloseOrder(ctx context.Context, req app.CloseOrderRequest) (*wechatcore.APIResult, error)
}

type JSAPIService interface {
	PrepayWithRequestPayment(ctx context.Context, req jsapi.PrepayRequest) (*jsapi.PrepayWithRequestPaymentResponse, *wechatcore.APIResult, error)
	QueryOrderById(ctx context.Context, req jsapi.QueryOrderByIdRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error)
	QueryOrderByOutTradeNo(ctx context.Context, req jsapi.QueryOrderByOutTradeNoRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error)
	CloseOrder(ctx context.Context, req jsapi.CloseOrderRequest) (*wechatcore.APIResult, error)
}

type H5Service interface {
	Prepay(ctx context.Context, req h5.PrepayRequest) (*h5.PrepayResponse, *wechatcore.APIResult, error)
	QueryOrderById(ctx context.Context, req h5.QueryOrderByIdRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error)
	QueryOrderByOutTradeNo(ctx context.Context, req h5.QueryOrderByOutTradeNoRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error)
	CloseOrder(ctx context.Context, req h5.CloseOrderRequest) (*wechatcore.APIResult, error)
}

type NativeService interface {
	Prepay(ctx context.Context, req native.PrepayRequest) (*native.PrepayResponse, *wechatcore.APIResult, error)
	QueryOrderById(ctx context.Context, req native.QueryOrderByIdRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error)
	QueryOrderByOutTradeNo(ctx context.Context, req native.QueryOrderByOutTradeNoRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error)
	CloseOrder(ctx context.Context, req native.CloseOrderRequest) (*wechatcore.APIResult, error)
}

type RefundService interface {
	Create(ctx context.Context, req refunddomestic.CreateRequest) (*refunddomestic.Refund, *wechatcore.APIResult, error)
	QueryByOutRefundNo(ctx context.Context, req refunddomestic.QueryByOutRefundNoRequest) (*refunddomestic.Refund, *wechatcore.APIResult, error)
}

type Config struct {
	AppID            string
	MerchantID       string
	MerchantSerialNo string
	NotifyURL        string
	PrivateKey       *rsa.PrivateKey
	APIv3Key         string
	HTTPClient       *http.Client
	CoreClient       *wechatcore.Client

	AppService    AppService
	JSAPIService  JSAPIService
	H5Service     H5Service
	NativeService NativeService
	RefundService RefundService
}

type Client struct {
	appID      string
	merchantID string
	notifyURL  string

	appService    AppService
	jsapiService  JSAPIService
	h5Service     H5Service
	nativeService NativeService
	refundService RefundService
}
