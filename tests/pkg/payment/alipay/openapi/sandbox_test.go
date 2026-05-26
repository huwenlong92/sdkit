package openapi_test

import (
	"context"
	"html/template"
	"os"
	"strings"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/alipay/openapi"
)

const defaultAlipaySandboxGatewayURL = "https://openapi-sandbox.dl.alipaydev.com/gateway.do"

func TestSandboxConfigBuildsPagePayRequest(t *testing.T) {
	client, gatewayURL, appID := newSandboxClient(t)

	resp, err := client.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderAlipay,
		Channel:    payment.ChannelAlipayPage,
		OutTradeNo: firstNonEmpty(os.Getenv("SDKIT_ALIPAY_OUT_TRADE_NO"), "sdkit_sandbox_202605260001"),
		Subject:    "sdkit sandbox payment",
		Pricing:    payment.CNY(1),
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if resp.Action.Type != payment.ActionHTMLForm || resp.Action.URL != gatewayURL {
		t.Fatalf("action = %+v", resp.Action)
	}
	if resp.Action.Fields["app_id"] != appID ||
		resp.Action.Fields["method"] != "alipay.trade.page.pay" ||
		resp.Action.Fields["sign"] == "" {
		t.Fatalf("fields = %+v", resp.Action.Fields)
	}
	if path := strings.TrimSpace(os.Getenv("SDKIT_ALIPAY_FORM_HTML")); path != "" {
		if err := writeHTMLForm(path, resp.Action.URL, resp.Action.Fields); err != nil {
			t.Fatalf("write html form: %v", err)
		}
		t.Logf("wrote alipay sandbox form: %s", path)
	}
}

func TestSandboxQueryPayment(t *testing.T) {
	client, _, _ := newSandboxClient(t)
	outTradeNo := strings.TrimSpace(os.Getenv("SDKIT_ALIPAY_QUERY_OUT_TRADE_NO"))
	tradeNo := strings.TrimSpace(os.Getenv("SDKIT_ALIPAY_QUERY_TRADE_NO"))
	if outTradeNo == "" && tradeNo == "" {
		t.Skip("SDKIT_ALIPAY_QUERY_OUT_TRADE_NO or SDKIT_ALIPAY_QUERY_TRADE_NO is required")
	}

	resp, err := client.QueryPayment(context.Background(), payment.QueryPaymentRequest{
		Provider:        payment.ProviderAlipay,
		Channel:         payment.ChannelAlipayPage,
		OutTradeNo:      outTradeNo,
		ProviderTradeID: tradeNo,
	})
	if err != nil {
		t.Fatalf("query payment: %v", err)
	}
	t.Logf("alipay query status=%s out_trade_no=%s trade_no=%s amount=%d", resp.Status, resp.OutTradeNo, resp.ProviderTradeID, resp.Pricing.PayAmount.Amount)
	if resp.Status == "" {
		t.Fatalf("status is empty: %+v", resp)
	}
}

func TestSandboxRefund(t *testing.T) {
	client, _, _ := newSandboxClient(t)
	outTradeNo := strings.TrimSpace(os.Getenv("SDKIT_ALIPAY_REFUND_OUT_TRADE_NO"))
	tradeNo := strings.TrimSpace(os.Getenv("SDKIT_ALIPAY_REFUND_TRADE_NO"))
	if outTradeNo == "" && tradeNo == "" {
		t.Skip("SDKIT_ALIPAY_REFUND_OUT_TRADE_NO or SDKIT_ALIPAY_REFUND_TRADE_NO is required")
	}
	outRefundNo := firstNonEmpty(os.Getenv("SDKIT_ALIPAY_REFUND_OUT_REQUEST_NO"), "sdkitrefund202605260001")

	resp, err := client.Refund(context.Background(), payment.RefundRequest{
		Provider:        payment.ProviderAlipay,
		Channel:         payment.ChannelAlipayPage,
		OutTradeNo:      outTradeNo,
		ProviderTradeID: tradeNo,
		RefundID:        outRefundNo,
		Amount: payment.RefundAmount{
			Refund: payment.CNY(1).PayAmount,
		},
		Reason: "sdkit sandbox refund",
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	t.Logf("alipay refund status=%s out_trade_no=%s trade_no=%s out_request_no=%s amount=%d", resp.Status, resp.OutTradeNo, resp.ProviderTradeID, resp.OutRefundNo, resp.Amount.Refund.Amount)
	if resp.Status != payment.RefundSucceeded {
		t.Fatalf("refund status = %s", resp.Status)
	}
}

func TestSandboxQueryRefund(t *testing.T) {
	client, _, _ := newSandboxClient(t)
	outTradeNo := strings.TrimSpace(os.Getenv("SDKIT_ALIPAY_REFUND_QUERY_OUT_TRADE_NO"))
	tradeNo := strings.TrimSpace(os.Getenv("SDKIT_ALIPAY_REFUND_QUERY_TRADE_NO"))
	if outTradeNo == "" && tradeNo == "" {
		t.Skip("SDKIT_ALIPAY_REFUND_QUERY_OUT_TRADE_NO or SDKIT_ALIPAY_REFUND_QUERY_TRADE_NO is required")
	}
	outRefundNo := firstNonEmpty(os.Getenv("SDKIT_ALIPAY_REFUND_QUERY_OUT_REQUEST_NO"), "sdkitrefund202605260001")

	resp, err := client.QueryRefund(context.Background(), payment.QueryRefundRequest{
		Provider:        payment.ProviderAlipay,
		Channel:         payment.ChannelAlipayPage,
		OutTradeNo:      outTradeNo,
		ProviderTradeID: tradeNo,
		OutRefundNo:     outRefundNo,
	})
	if err != nil {
		t.Fatalf("query refund: %v", err)
	}
	t.Logf("alipay refund query status=%s out_request_no=%s trade_no=%s amount=%d", resp.Status, resp.OutRefundNo, resp.ProviderTradeID, resp.Amount.Refund.Amount)
	if resp.Status == "" {
		t.Fatalf("status is empty: %+v", resp)
	}
}

func newSandboxClient(t *testing.T) (*openapi.Client, string, string) {
	t.Helper()
	if os.Getenv("SDKIT_ALIPAY_SANDBOX") != "1" {
		t.Skip("set SDKIT_ALIPAY_SANDBOX=1 and sandbox credentials to enable")
	}

	appID := strings.TrimSpace(os.Getenv("SDKIT_ALIPAY_APP_ID"))
	privateKeyPEM := normalizePEM(os.Getenv("SDKIT_ALIPAY_PRIVATE_KEY_PEM"))
	if appID == "" || privateKeyPEM == "" {
		t.Skip("SDKIT_ALIPAY_APP_ID and SDKIT_ALIPAY_PRIVATE_KEY_PEM are required")
	}

	signer, err := openapi.NewRSASignerFromPEM([]byte(privateKeyPEM))
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	gatewayURL := strings.TrimSpace(os.Getenv("SDKIT_ALIPAY_GATEWAY_URL"))
	if gatewayURL == "" {
		gatewayURL = defaultAlipaySandboxGatewayURL
	}
	client, err := openapi.NewClient(openapi.Config{
		AppID:      appID,
		GatewayURL: gatewayURL,
		NotifyURL:  strings.TrimSpace(os.Getenv("SDKIT_ALIPAY_NOTIFY_URL")),
		ReturnURL:  strings.TrimSpace(os.Getenv("SDKIT_ALIPAY_RETURN_URL")),
		Signer:     signer,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client, gatewayURL, appID
}

func normalizePEM(raw string) string {
	return strings.ReplaceAll(strings.TrimSpace(raw), `\n`, "\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func writeHTMLForm(path, actionURL string, fields map[string]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	tpl := template.Must(template.New("form").Parse(`<!doctype html>
<html>
<head><meta charset="utf-8"><title>Alipay Sandbox</title></head>
<body>
<form id="pay" method="post" action="{{.Action}}">
{{range $key, $value := .Fields}}<input type="hidden" name="{{$key}}" value="{{$value}}">
{{end}}</form>
<script>document.getElementById("pay").submit();</script>
</body>
</html>
`))
	return tpl.Execute(file, struct {
		Action string
		Fields map[string]string
	}{
		Action: actionURL,
		Fields: fields,
	})
}
