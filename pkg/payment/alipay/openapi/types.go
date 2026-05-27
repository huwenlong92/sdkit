//go:build sdkit_payment_alipay

package openapi

import (
	"context"
	"net/http"
	"time"
)

const DefaultGatewayURL = "https://openapi.alipay.com/gateway.do"

type Signer interface {
	Sign(ctx context.Context, content string) (string, error)
}

type Config struct {
	AppID      string
	GatewayURL string
	NotifyURL  string
	ReturnURL  string
	Signer     Signer
	HTTPClient *http.Client
	Clock      func() time.Time
}

type Client struct {
	appID      string
	gatewayURL string
	notifyURL  string
	returnURL  string
	signer     Signer
	httpClient *http.Client
	clock      func() time.Time
}
