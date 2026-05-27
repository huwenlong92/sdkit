//go:build sdkit_payment_paypal

package ordersapi

import (
	"net/http"
	"sync"
	"time"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/debuglog"
)

const (
	DefaultSandboxBaseURL = "https://api-m.sandbox.paypal.com"
	DefaultLiveBaseURL    = "https://api-m.paypal.com"

	ExtraCancelURLKey = "cancel_url"
	ExtraOrderIDKey   = "paypal_order_id"
	ExtraCaptureIDKey = "paypal_capture_id"
)

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type Config struct {
	ClientID         string
	ClientSecret     string
	BaseURL          string
	ReturnURL        string
	CancelURL        string
	HTTPClient       HTTPDoer
	Clock            func() time.Time
	DebugLogger      debuglog.Logger
	DebugPayloadMode debuglog.PayloadMode
}

type Client struct {
	clientID         string
	clientSecret     string
	baseURL          string
	returnURL        string
	cancelURL        string
	httpClient       HTTPDoer
	clock            func() time.Time
	debugLogger      debuglog.Logger
	debugPayloadMode debuglog.PayloadMode

	tokenMu sync.Mutex
	token   oauthToken
}

type CapturePaymentRequest struct {
	Provider        payment.Provider
	Channel         payment.Channel
	MerchantKey     string
	PaymentID       string
	OrderID         string
	OutTradeNo      string
	ProviderTradeID string
	Extra           map[string]any
}

type oauthToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	expiresAt   time.Time
}

type amountPayload struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type createOrderRequest struct {
	Intent        string                 `json:"intent"`
	PurchaseUnits []purchaseUnit         `json:"purchase_units"`
	PaymentSource *paymentSourcePayload  `json:"payment_source,omitempty"`
	Application   map[string]interface{} `json:"application_context,omitempty"`
}

type paymentSourcePayload struct {
	PayPal paypalPaymentSource `json:"paypal"`
}

type paypalPaymentSource struct {
	ExperienceContext paypalExperienceContext `json:"experience_context"`
}

type paypalExperienceContext struct {
	ReturnURL  string `json:"return_url,omitempty"`
	CancelURL  string `json:"cancel_url,omitempty"`
	UserAction string `json:"user_action,omitempty"`
}

type purchaseUnit struct {
	ReferenceID string        `json:"reference_id,omitempty"`
	Description string        `json:"description,omitempty"`
	CustomID    string        `json:"custom_id,omitempty"`
	InvoiceID   string        `json:"invoice_id,omitempty"`
	Amount      amountPayload `json:"amount"`
}

type orderResponse struct {
	ID            string         `json:"id"`
	Status        string         `json:"status"`
	PurchaseUnits []purchaseUnit `json:"purchase_units"`
	Links         []linkPayload  `json:"links"`
}

type captureOrderResponse struct {
	ID            string                `json:"id"`
	Status        string                `json:"status"`
	PurchaseUnits []capturePurchaseUnit `json:"purchase_units"`
	Links         []linkPayload         `json:"links"`
}

type capturePurchaseUnit struct {
	ReferenceID string          `json:"reference_id"`
	Payments    capturePayments `json:"payments"`
}

type capturePayments struct {
	Captures []capturePayload `json:"captures"`
}

type capturePayload struct {
	ID     string        `json:"id"`
	Status string        `json:"status"`
	Amount amountPayload `json:"amount"`
}

type refundRequest struct {
	Amount amountPayload `json:"amount"`
	Note   string        `json:"note_to_payer,omitempty"`
}

type refundResponse struct {
	ID     string        `json:"id"`
	Status string        `json:"status"`
	Amount amountPayload `json:"amount"`
	Links  []linkPayload `json:"links"`
}

type linkPayload struct {
	Href   string `json:"href"`
	Rel    string `json:"rel"`
	Method string `json:"method"`
}
