package apiv3_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/wechat/apiv3"
	wechatcore "github.com/wechatpay-apiv3/wechatpay-go/core"
	wechatpayments "github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/app"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/h5"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/jsapi"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/services/refunddomestic"
)

type fakeWechatServices struct {
	appReq    app.PrepayRequest
	jsapiReq  jsapi.PrepayRequest
	h5Req     h5.PrepayRequest
	nativeReq native.PrepayRequest

	appQueryReq    app.QueryOrderByOutTradeNoRequest
	appCloseReq    app.CloseOrderRequest
	refundReq      refunddomestic.CreateRequest
	refundQueryReq refunddomestic.QueryByOutRefundNoRequest
}

type fakeAppService struct {
	base *fakeWechatServices
}

func (s fakeAppService) PrepayWithRequestPayment(_ context.Context, req app.PrepayRequest) (*app.PrepayWithRequestPaymentResponse, *wechatcore.APIResult, error) {
	s.base.appReq = req
	return appPaymentResponse(), nil, nil
}

func (s fakeAppService) QueryOrderById(_ context.Context, req app.QueryOrderByIdRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error) {
	return transaction(), nil, nil
}

func (s fakeAppService) QueryOrderByOutTradeNo(_ context.Context, req app.QueryOrderByOutTradeNoRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error) {
	s.base.appQueryReq = req
	return transaction(), nil, nil
}

func (s fakeAppService) CloseOrder(_ context.Context, req app.CloseOrderRequest) (*wechatcore.APIResult, error) {
	s.base.appCloseReq = req
	return nil, nil
}

type fakeJSAPIService struct {
	base *fakeWechatServices
}

func (s fakeJSAPIService) PrepayWithRequestPayment(ctx context.Context, req jsapi.PrepayRequest) (*jsapi.PrepayWithRequestPaymentResponse, *wechatcore.APIResult, error) {
	s.base.jsapiReq = req
	return &jsapi.PrepayWithRequestPaymentResponse{
		PrepayId:  stringPtr("wx_prepay_jsapi"),
		Appid:     stringPtr("wx_app_1"),
		TimeStamp: stringPtr("1779696000"),
		NonceStr:  stringPtr("nonce_2"),
		Package:   stringPtr("prepay_id=wx_prepay_jsapi"),
		SignType:  stringPtr("RSA"),
		PaySign:   stringPtr("signed_jsapi"),
	}, nil, nil
}

func (s fakeJSAPIService) QueryOrderById(_ context.Context, req jsapi.QueryOrderByIdRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error) {
	return transaction(), nil, nil
}

func (s fakeJSAPIService) QueryOrderByOutTradeNo(_ context.Context, req jsapi.QueryOrderByOutTradeNoRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error) {
	return transaction(), nil, nil
}

func (s fakeJSAPIService) CloseOrder(_ context.Context, req jsapi.CloseOrderRequest) (*wechatcore.APIResult, error) {
	return nil, nil
}

type fakeH5Service struct {
	base *fakeWechatServices
}

func (s fakeH5Service) Prepay(_ context.Context, req h5.PrepayRequest) (*h5.PrepayResponse, *wechatcore.APIResult, error) {
	s.base.h5Req = req
	return &h5.PrepayResponse{H5Url: stringPtr("https://wxpay.example.test/h5")}, nil, nil
}

func (s fakeH5Service) QueryOrderById(_ context.Context, req h5.QueryOrderByIdRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error) {
	return transaction(), nil, nil
}

func (s fakeH5Service) QueryOrderByOutTradeNo(_ context.Context, req h5.QueryOrderByOutTradeNoRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error) {
	return transaction(), nil, nil
}

func (s fakeH5Service) CloseOrder(_ context.Context, req h5.CloseOrderRequest) (*wechatcore.APIResult, error) {
	return nil, nil
}

type fakeNativeService struct {
	base *fakeWechatServices
}

func (s fakeNativeService) Prepay(ctx context.Context, req native.PrepayRequest) (*native.PrepayResponse, *wechatcore.APIResult, error) {
	s.base.nativeReq = req
	return &native.PrepayResponse{CodeUrl: stringPtr("weixin://wxpay/bizpayurl?pr=abc")}, nil, nil
}

func (s fakeNativeService) QueryOrderById(_ context.Context, req native.QueryOrderByIdRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error) {
	return transaction(), nil, nil
}

func (s fakeNativeService) QueryOrderByOutTradeNo(_ context.Context, req native.QueryOrderByOutTradeNoRequest) (*wechatpayments.Transaction, *wechatcore.APIResult, error) {
	return transaction(), nil, nil
}

func (s fakeNativeService) CloseOrder(_ context.Context, req native.CloseOrderRequest) (*wechatcore.APIResult, error) {
	return nil, nil
}

type fakeRefundService struct {
	base *fakeWechatServices
}

func (s fakeRefundService) Create(_ context.Context, req refunddomestic.CreateRequest) (*refunddomestic.Refund, *wechatcore.APIResult, error) {
	s.base.refundReq = req
	return refund("SUCCESS"), nil, nil
}

func (s fakeRefundService) QueryByOutRefundNo(_ context.Context, req refunddomestic.QueryByOutRefundNoRequest) (*refunddomestic.Refund, *wechatcore.APIResult, error) {
	s.base.refundQueryReq = req
	return refund("SUCCESS"), nil, nil
}

func TestClientCreatePaymentChannels(t *testing.T) {
	fake := &fakeWechatServices{}
	client, err := apiv3.NewClient(apiv3.Config{
		AppID:         "wx_app_1",
		MerchantID:    "mch_1",
		NotifyURL:     "https://example.test/pay/notify/wechat",
		AppService:    fakeAppService{base: fake},
		JSAPIService:  fakeJSAPIService{base: fake},
		H5Service:     fakeH5Service{base: fake},
		NativeService: fakeNativeService{base: fake},
		RefundService: fakeRefundService{base: fake},
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	appResp, err := client.CreatePayment(context.Background(), createRequest(payment.ChannelWechatApp, nil))
	if err != nil {
		t.Fatalf("create app payment: %v", err)
	}
	if value(fake.appReq.Appid) != "wx_app_1" || value(fake.appReq.Mchid) != "mch_1" || value(fake.appReq.OutTradeNo) != "pay_1001" {
		t.Fatalf("app request = %+v", fake.appReq)
	}
	if fake.appReq.Amount == nil || valueInt64(fake.appReq.Amount.Total) != 19900 || value(fake.appReq.Amount.Currency) != "CNY" {
		t.Fatalf("app amount = %+v", fake.appReq.Amount)
	}
	if appResp.Action.Type != payment.ActionSDKParams ||
		appResp.Action.Params["appid"] != "wx_app_1" ||
		appResp.Action.Params["partnerid"] != "mch_1" ||
		appResp.Action.Params["prepayid"] != "wx_prepay_app" {
		t.Fatalf("app action = %+v", appResp.Action)
	}

	jsapiResp, err := client.CreatePayment(context.Background(), createRequest(payment.ChannelWechatMiniProgram, map[string]any{
		apiv3.ExtraOpenIDKey: "openid_1",
	}))
	if err != nil {
		t.Fatalf("create jsapi payment: %v", err)
	}
	if fake.jsapiReq.Payer == nil || value(fake.jsapiReq.Payer.Openid) != "openid_1" {
		t.Fatalf("jsapi payer = %+v", fake.jsapiReq.Payer)
	}
	if jsapiResp.Action.Type != payment.ActionSDKParams ||
		jsapiResp.Action.Params["package"] != "prepay_id=wx_prepay_jsapi" ||
		jsapiResp.Action.Params["paySign"] != "signed_jsapi" {
		t.Fatalf("jsapi action = %+v", jsapiResp.Action)
	}

	h5Resp, err := client.CreatePayment(context.Background(), createRequest(payment.ChannelWechatH5, map[string]any{
		apiv3.ExtraClientIPKey: "127.0.0.1",
	}))
	if err != nil {
		t.Fatalf("create h5 payment: %v", err)
	}
	if fake.h5Req.SceneInfo == nil ||
		value(fake.h5Req.SceneInfo.PayerClientIp) != "127.0.0.1" ||
		fake.h5Req.SceneInfo.H5Info == nil ||
		value(fake.h5Req.SceneInfo.H5Info.Type) != "Wap" {
		t.Fatalf("h5 scene info = %+v", fake.h5Req.SceneInfo)
	}
	if h5Resp.Action.Type != payment.ActionRedirectURL || h5Resp.Action.URL != "https://wxpay.example.test/h5" {
		t.Fatalf("h5 action = %+v", h5Resp.Action)
	}

	nativeResp, err := client.CreatePayment(context.Background(), createRequest(payment.ChannelWechatNative, nil))
	if err != nil {
		t.Fatalf("create native payment: %v", err)
	}
	if value(fake.nativeReq.OutTradeNo) != "pay_1001" {
		t.Fatalf("native request = %+v", fake.nativeReq)
	}
	if nativeResp.Action.Type != payment.ActionQRCode || nativeResp.Action.URL != "weixin://wxpay/bizpayurl?pr=abc" {
		t.Fatalf("native action = %+v", nativeResp.Action)
	}
}

func TestClientRequiresMiniProgramOpenID(t *testing.T) {
	client := newClientForValidation(t)

	_, err := client.CreatePayment(context.Background(), createRequest(payment.ChannelWechatMiniProgram, nil))
	if err == nil {
		t.Fatalf("err = nil, want error")
	}
}

func TestClientRequiresH5ClientIP(t *testing.T) {
	client := newClientForValidation(t)

	_, err := client.CreatePayment(context.Background(), createRequest(payment.ChannelWechatH5, nil))
	if err == nil {
		t.Fatalf("err = nil, want error")
	}
}

func TestClientRequiresAPIv3KeyWhenUsingOfficialServices(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	_, err = apiv3.NewClient(apiv3.Config{
		AppID:            "wx_app_1",
		MerchantID:       "mch_1",
		MerchantSerialNo: "serial_1",
		NotifyURL:        "https://example.test/pay/notify/wechat",
		PrivateKey:       privateKey,
	})
	if err == nil {
		t.Fatalf("err = nil, want error")
	}
}

func TestClientQueryCloseRefundAndQueryRefund(t *testing.T) {
	fake := &fakeWechatServices{}
	client, err := apiv3.NewClient(apiv3.Config{
		AppID:         "wx_app_1",
		MerchantID:    "mch_1",
		NotifyURL:     "https://example.test/pay/notify/wechat",
		AppService:    fakeAppService{base: fake},
		JSAPIService:  fakeJSAPIService{base: fake},
		H5Service:     fakeH5Service{base: fake},
		NativeService: fakeNativeService{base: fake},
		RefundService: fakeRefundService{base: fake},
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	queryResp, err := client.QueryPayment(context.Background(), payment.QueryPaymentRequest{
		Provider:   payment.ProviderWechat,
		Channel:    payment.ChannelWechatApp,
		OutTradeNo: "pay_1001",
	})
	if err != nil {
		t.Fatalf("query payment: %v", err)
	}
	if value(fake.appQueryReq.OutTradeNo) != "pay_1001" || value(fake.appQueryReq.Mchid) != "mch_1" {
		t.Fatalf("query req = %+v", fake.appQueryReq)
	}
	if queryResp.Status != payment.PaymentSucceeded ||
		queryResp.ProviderTradeID != "wx_tx_1001" ||
		queryResp.Pricing.PayAmount.Amount != 19900 {
		t.Fatalf("query resp = %+v", queryResp)
	}

	if err := client.ClosePayment(context.Background(), payment.ClosePaymentRequest{
		Provider:   payment.ProviderWechat,
		Channel:    payment.ChannelWechatApp,
		OutTradeNo: "pay_1001",
	}); err != nil {
		t.Fatalf("close payment: %v", err)
	}
	if value(fake.appCloseReq.OutTradeNo) != "pay_1001" || value(fake.appCloseReq.Mchid) != "mch_1" {
		t.Fatalf("close req = %+v", fake.appCloseReq)
	}

	refundResp, err := client.Refund(context.Background(), payment.RefundRequest{
		Provider:   payment.ProviderWechat,
		Channel:    payment.ChannelWechatApp,
		OutTradeNo: "pay_1001",
		RefundID:   "refund_1001",
		Amount: payment.RefundAmount{
			Refund: payment.Money{Amount: 100, Currency: payment.DefaultCurrency},
		},
		Reason: "测试退款",
		Extra: map[string]any{
			apiv3.ExtraRefundTotalAmountKey: int64(19900),
		},
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if value(fake.refundReq.OutTradeNo) != "pay_1001" ||
		value(fake.refundReq.OutRefundNo) != "refund_1001" ||
		valueInt64(fake.refundReq.Amount.Refund) != 100 ||
		valueInt64(fake.refundReq.Amount.Total) != 19900 {
		t.Fatalf("refund req = %+v", fake.refundReq)
	}
	if refundResp.Status != payment.RefundSucceeded ||
		refundResp.ProviderRefundID != "wx_refund_1001" ||
		refundResp.Amount.Refund.Amount != 100 {
		t.Fatalf("refund resp = %+v", refundResp)
	}

	queryRefundResp, err := client.QueryRefund(context.Background(), payment.QueryRefundRequest{
		Provider:    payment.ProviderWechat,
		Channel:     payment.ChannelWechatApp,
		OutRefundNo: "refund_1001",
	})
	if err != nil {
		t.Fatalf("query refund: %v", err)
	}
	if value(fake.refundQueryReq.OutRefundNo) != "refund_1001" {
		t.Fatalf("refund query req = %+v", fake.refundQueryReq)
	}
	if queryRefundResp.Status != payment.RefundSucceeded ||
		queryRefundResp.ProviderRefundID != "wx_refund_1001" {
		t.Fatalf("query refund resp = %+v", queryRefundResp)
	}
}

func TestClientRefundRequiresTotalAmount(t *testing.T) {
	client := newClientForValidation(t)

	_, err := client.Refund(context.Background(), payment.RefundRequest{
		Provider:   payment.ProviderWechat,
		Channel:    payment.ChannelWechatApp,
		OutTradeNo: "pay_1001",
		RefundID:   "refund_1001",
		Amount: payment.RefundAmount{
			Refund: payment.Money{Amount: 100, Currency: payment.DefaultCurrency},
		},
	})
	if err == nil {
		t.Fatalf("err = nil, want error")
	}
}

func newClientForValidation(t *testing.T) *apiv3.Client {
	t.Helper()
	fake := &fakeWechatServices{}
	client, err := apiv3.NewClient(apiv3.Config{
		AppID:         "wx_app_1",
		MerchantID:    "mch_1",
		NotifyURL:     "https://example.test/pay/notify/wechat",
		AppService:    fakeAppService{base: fake},
		JSAPIService:  fakeJSAPIService{base: fake},
		H5Service:     fakeH5Service{base: fake},
		NativeService: fakeNativeService{base: fake},
		RefundService: fakeRefundService{base: fake},
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func createRequest(channel payment.Channel, extra map[string]any) payment.CreatePaymentRequest {
	return payment.CreatePaymentRequest{
		Provider:   payment.ProviderWechat,
		Channel:    channel,
		OrderID:    "order_1001",
		OutTradeNo: "pay_1001",
		Subject:    "会员年卡",
		Pricing:    payment.CNY(19900),
		Extra:      extra,
	}
}

func stringPtr(value string) *string {
	return &value
}

func value(in *string) string {
	if in == nil {
		return ""
	}
	return *in
}

func valueInt64(in *int64) int64 {
	if in == nil {
		return 0
	}
	return *in
}

func appPaymentResponse() *app.PrepayWithRequestPaymentResponse {
	return &app.PrepayWithRequestPaymentResponse{
		PrepayId:  stringPtr("wx_prepay_app"),
		PartnerId: stringPtr("mch_1"),
		TimeStamp: stringPtr("1779696000"),
		NonceStr:  stringPtr("nonce_1"),
		Package:   stringPtr("Sign=WXPay"),
		Sign:      stringPtr("signed_app"),
	}
}

func transaction() *wechatpayments.Transaction {
	return &wechatpayments.Transaction{
		Amount: &wechatpayments.TransactionAmount{
			Total:    int64Ptr(19900),
			Currency: stringPtr(payment.DefaultCurrency),
		},
		OutTradeNo:     stringPtr("pay_1001"),
		TransactionId:  stringPtr("wx_tx_1001"),
		TradeState:     stringPtr("SUCCESS"),
		TradeStateDesc: stringPtr("支付成功"),
		TradeType:      stringPtr("APP"),
		SuccessTime:    stringPtr("2026-05-26T10:20:30+08:00"),
	}
}

func refund(status string) *refunddomestic.Refund {
	now := time.Date(2026, 5, 26, 10, 20, 30, 0, time.FixedZone("CST", 8*60*60))
	officialStatus := refunddomestic.Status(status)
	return &refunddomestic.Refund{
		RefundId:            stringPtr("wx_refund_1001"),
		OutRefundNo:         stringPtr("refund_1001"),
		TransactionId:       stringPtr("wx_tx_1001"),
		OutTradeNo:          stringPtr("pay_1001"),
		UserReceivedAccount: stringPtr("支付用户零钱"),
		SuccessTime:         &now,
		CreateTime:          &now,
		Status:              &officialStatus,
		Amount: &refunddomestic.Amount{
			Total:            int64Ptr(19900),
			Refund:           int64Ptr(100),
			PayerTotal:       int64Ptr(19900),
			PayerRefund:      int64Ptr(100),
			SettlementRefund: int64Ptr(100),
			SettlementTotal:  int64Ptr(19900),
			DiscountRefund:   int64Ptr(0),
			Currency:         stringPtr(payment.DefaultCurrency),
		},
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
