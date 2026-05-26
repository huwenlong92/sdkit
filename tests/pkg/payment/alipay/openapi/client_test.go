package openapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/alipay/openapi"
)

type signer struct {
	content string
}

func (s *signer) Sign(_ context.Context, content string) (string, error) {
	s.content = content
	return "signed", nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestClientCreatePaymentChannels(t *testing.T) {
	for _, tt := range []struct {
		name       string
		channel    payment.Channel
		method     string
		actionType payment.ActionType
	}{
		{name: "app", channel: payment.ChannelAlipayApp, method: "alipay.trade.app.pay", actionType: payment.ActionSDKParams},
		{name: "wap", channel: payment.ChannelAlipayWap, method: "alipay.trade.wap.pay", actionType: payment.ActionHTMLForm},
		{name: "page", channel: payment.ChannelAlipayPage, method: "alipay.trade.page.pay", actionType: payment.ActionHTMLForm},
	} {
		t.Run(tt.name, func(t *testing.T) {
			signer := &signer{}
			client, err := openapi.NewClient(openapi.Config{
				AppID:      "ali_app_1",
				GatewayURL: "https://openapi.alipay.example.test/gateway.do",
				NotifyURL:  "https://example.test/pay/notify/alipay",
				ReturnURL:  "https://example.test/pay/return",
				Signer:     signer,
				Clock:      func() time.Time { return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC) },
			})
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			expireAt := time.Date(2026, 5, 25, 12, 30, 0, 0, time.UTC)

			resp, err := client.CreatePayment(context.Background(), payment.CreatePaymentRequest{
				Provider:   payment.ProviderAlipay,
				Channel:    tt.channel,
				OrderID:    "order_1001",
				OutTradeNo: "pay_1001",
				Subject:    "会员年卡",
				Body:       "年卡",
				Pricing:    payment.CNY(19900),
				ExpireAt:   &expireAt,
			})
			if err != nil {
				t.Fatalf("create payment: %v", err)
			}
			if resp.Action.Type != tt.actionType {
				t.Fatalf("action type = %q, want %q", resp.Action.Type, tt.actionType)
			}

			fields := actionFields(t, resp.Action)
			if fields["app_id"] != "ali_app_1" || fields["method"] != tt.method || fields["sign"] != "signed" {
				t.Fatalf("fields = %+v", fields)
			}
			if fields["notify_url"] != "https://example.test/pay/notify/alipay" || fields["return_url"] != "https://example.test/pay/return" {
				t.Fatalf("urls = %+v", fields)
			}
			var biz map[string]string
			if err := json.Unmarshal([]byte(fields["biz_content"]), &biz); err != nil {
				t.Fatalf("decode biz_content: %v", err)
			}
			if biz["out_trade_no"] != "pay_1001" || biz["total_amount"] != "199.00" || biz["subject"] != "会员年卡" {
				t.Fatalf("biz = %+v", biz)
			}
			if biz["timeout_express"] != "30m" {
				t.Fatalf("timeout_express = %q", biz["timeout_express"])
			}
			if !strings.Contains(signer.content, "app_id=ali_app_1") ||
				!strings.Contains(signer.content, "method="+tt.method) ||
				strings.Contains(signer.content, "sign=") {
				t.Fatalf("sign content = %s", signer.content)
			}
		})
	}
}

func TestClientRejectsUnsupportedCurrency(t *testing.T) {
	client := newClientForValidation(t)

	_, err := client.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderAlipay,
		Channel:    payment.ChannelAlipayApp,
		OutTradeNo: "pay_1001",
		Subject:    "会员年卡",
		Pricing: payment.PaymentPricing{
			PayAmount: payment.Money{Amount: 100, Currency: "EUR"},
		},
	})
	if err == nil {
		t.Fatalf("err = nil, want error")
	}
}

func TestClientRejectsUnsupportedChannel(t *testing.T) {
	client := newClientForValidation(t)

	_, err := client.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderAlipay,
		Channel:    payment.ChannelWechatApp,
		OutTradeNo: "pay_1001",
		Subject:    "会员年卡",
		Pricing:    payment.CNY(100),
	})
	if err == nil {
		t.Fatalf("err = nil, want error")
	}
}

func TestClientQueryPayment(t *testing.T) {
	var gotForm url.Values
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		gotForm, err = url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse form: %v", err)
		}
		return httpResponse(`{
			"alipay_trade_query_response": {
				"code": "10000",
				"msg": "Success",
				"trade_no": "2026052622000000000001",
				"out_trade_no": "pay_1001",
				"trade_status": "TRADE_SUCCESS",
				"total_amount": "199.00",
				"send_pay_date": "2026-05-26 10:20:30"
			},
			"sign": "response-sign"
		}`), nil
	})}

	client, err := openapi.NewClient(openapi.Config{
		AppID:      "ali_app_1",
		GatewayURL: "https://openapi.alipay.example.test/gateway.do",
		Signer:     &signer{},
		HTTPClient: httpClient,
		Clock:      func() time.Time { return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	resp, err := client.QueryPayment(context.Background(), payment.QueryPaymentRequest{
		Provider:   payment.ProviderAlipay,
		Channel:    payment.ChannelAlipayPage,
		OrderID:    "order_1001",
		OutTradeNo: "pay_1001",
	})
	if err != nil {
		t.Fatalf("query payment: %v", err)
	}
	if gotForm.Get("method") != "alipay.trade.query" || gotForm.Get("app_id") != "ali_app_1" || gotForm.Get("sign") != "signed" {
		t.Fatalf("form = %+v", gotForm)
	}
	var biz map[string]string
	if err := json.Unmarshal([]byte(gotForm.Get("biz_content")), &biz); err != nil {
		t.Fatalf("decode biz: %v", err)
	}
	if biz["out_trade_no"] != "pay_1001" {
		t.Fatalf("biz = %+v", biz)
	}
	if resp.Status != payment.PaymentSucceeded ||
		resp.ProviderTradeID != "2026052622000000000001" ||
		resp.Pricing.PayAmount.Amount != 19900 {
		t.Fatalf("response = %+v", resp)
	}
}

func TestClientRefund(t *testing.T) {
	var gotForm url.Values
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		gotForm, err = url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse form: %v", err)
		}
		return httpResponse(`{
			"alipay_trade_refund_response": {
				"code": "10000",
				"msg": "Success",
				"trade_no": "2026052622000000000001",
				"out_trade_no": "pay_1001",
				"fund_change": "Y",
				"refund_fee": "1.00"
			},
			"sign": "response-sign"
		}`), nil
	})}

	client, err := openapi.NewClient(openapi.Config{
		AppID:      "ali_app_1",
		GatewayURL: "https://openapi.alipay.example.test/gateway.do",
		Signer:     &signer{},
		HTTPClient: httpClient,
		Clock:      func() time.Time { return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	resp, err := client.Refund(context.Background(), payment.RefundRequest{
		Provider:   payment.ProviderAlipay,
		Channel:    payment.ChannelAlipayPage,
		OutTradeNo: "pay_1001",
		RefundID:   "refund_1001",
		Amount: payment.RefundAmount{
			Refund: payment.Money{Amount: 100, Currency: payment.DefaultCurrency},
		},
		Reason: "测试退款",
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if gotForm.Get("method") != "alipay.trade.refund" || gotForm.Get("sign") != "signed" {
		t.Fatalf("form = %+v", gotForm)
	}
	var biz map[string]string
	if err := json.Unmarshal([]byte(gotForm.Get("biz_content")), &biz); err != nil {
		t.Fatalf("decode biz: %v", err)
	}
	if biz["out_trade_no"] != "pay_1001" ||
		biz["out_request_no"] != "refund_1001" ||
		biz["refund_amount"] != "1.00" ||
		biz["refund_reason"] != "测试退款" {
		t.Fatalf("biz = %+v", biz)
	}
	if resp.Status != payment.RefundSucceeded ||
		resp.OutTradeNo != "pay_1001" ||
		resp.OutRefundNo != "refund_1001" ||
		resp.Amount.Refund.Amount != 100 {
		t.Fatalf("response = %+v", resp)
	}
}

func TestClientQueryRefund(t *testing.T) {
	var gotForm url.Values
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		gotForm, err = url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse form: %v", err)
		}
		return httpResponse(`{
			"alipay_trade_fastpay_refund_query_response": {
				"code": "10000",
				"msg": "Success",
				"trade_no": "2026052622000000000001",
				"out_trade_no": "pay_1001",
				"out_request_no": "refund_1001",
				"refund_amount": "1.00",
				"refund_status": "REFUND_SUCCESS"
			},
			"sign": "response-sign"
		}`), nil
	})}

	client, err := openapi.NewClient(openapi.Config{
		AppID:      "ali_app_1",
		GatewayURL: "https://openapi.alipay.example.test/gateway.do",
		Signer:     &signer{},
		HTTPClient: httpClient,
		Clock:      func() time.Time { return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	resp, err := client.QueryRefund(context.Background(), payment.QueryRefundRequest{
		Provider:    payment.ProviderAlipay,
		Channel:     payment.ChannelAlipayPage,
		OutTradeNo:  "pay_1001",
		OutRefundNo: "refund_1001",
	})
	if err != nil {
		t.Fatalf("query refund: %v", err)
	}
	if gotForm.Get("method") != "alipay.trade.fastpay.refund.query" || gotForm.Get("sign") != "signed" {
		t.Fatalf("form = %+v", gotForm)
	}
	var biz map[string]string
	if err := json.Unmarshal([]byte(gotForm.Get("biz_content")), &biz); err != nil {
		t.Fatalf("decode biz: %v", err)
	}
	if biz["out_trade_no"] != "pay_1001" || biz["out_request_no"] != "refund_1001" {
		t.Fatalf("biz = %+v", biz)
	}
	if resp.Status != payment.RefundSucceeded ||
		resp.OutTradeNo != "pay_1001" ||
		resp.ProviderTradeID != "2026052622000000000001" ||
		resp.OutRefundNo != "refund_1001" ||
		resp.Amount.Refund.Amount != 100 {
		t.Fatalf("response = %+v", resp)
	}
}

func newClientForValidation(t *testing.T) *openapi.Client {
	t.Helper()
	client, err := openapi.NewClient(openapi.Config{
		AppID:  "ali_app_1",
		Signer: &signer{},
		Clock:  func() time.Time { return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func actionFields(t *testing.T, action payment.PaymentAction) map[string]string {
	t.Helper()
	if action.Type == payment.ActionHTMLForm {
		return action.Fields
	}
	raw := action.Params["order_string"].(string)
	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse order string: %v", err)
	}
	fields := make(map[string]string, len(values))
	for key := range values {
		fields[key] = values.Get(key)
	}
	return fields
}

func httpResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
