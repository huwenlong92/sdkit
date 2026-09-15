//go:build sdkit_payment_airwallex

package httpapi

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/airwallex"
	"github.com/huwenlong92/sdkit/pkg/request"
)

type Environment string

type AuthenticationMode string

const (
	// AuthenticateAccount preserves explicit account-scoped authentication.
	AuthenticateAccount AuthenticationMode = "account"
	// AuthenticateDefault is for a scoped key bound to exactly one account.
	// AccountID remains required for webhook identity verification.
	AuthenticateDefault AuthenticationMode = "default"
)

const (
	Sandbox           Environment = "sandbox"
	Production        Environment = "production"
	SandboxBaseURL                = "https://api.sandbox.airwallex.com"
	ProductionBaseURL             = "https://api.airwallex.com"
	ExtraRequestIDKey             = "request_id"
)

type Config struct {
	Environment        Environment
	ClientID           string
	APIKey             string
	AccountID          string
	AuthenticationMode AuthenticationMode
	WebhookSecret      string
	// NotifyURL is the endpoint already registered in Airwallex Webhooks.
	NotifyURL        string
	ReturnURL        string
	HTTPClient       *http.Client
	Clock            func() time.Time
	WebhookTolerance time.Duration
	// CurrencyMetadata extends core defaults; it is precision metadata, not an
	// assertion that this merchant has enabled a currency in Airwallex.
	CurrencyMetadata map[string]payment.CurrencyMeta
}
type Client struct {
	authenticationMode                                    AuthenticationMode
	environment                                           Environment
	clientID, apiKey, accountID, webhookSecret, returnURL string
	notifyURL                                             string
	transport                                             *request.Client
	clock                                                 func() time.Time
	tolerance                                             time.Duration
	currencies                                            map[string]payment.CurrencyMeta
	mu                                                    sync.Mutex
	token                                                 string
	expiresAt                                             time.Time
	refreshing                                            chan struct{}
}

var _ airwallex.Client = (*Client)(nil)

// APIError deliberately excludes upstream message, response body and credentials.
// Retry a failed mutation only with the original persisted request_id and payload.
type APIError struct {
	StatusCode int
	Code       string
}

func (e *APIError) Error() string { return "airwallex API request failed (see StatusCode and Code)" }

type intent struct {
	RawBody              []byte `json:"-"`
	LatestPaymentAttempt *struct {
		ID            string `json:"id"`
		TransactionID string `json:"payment_method_transaction_id"`
		PaymentMethod struct {
			Type string `json:"type"`
			Card *struct {
				Brand string `json:"brand"`
				Last4 string `json:"last4"`
			} `json:"card"`
		} `json:"payment_method"`
	} `json:"latest_payment_attempt"`
	ID              string      `json:"id"`
	RequestID       string      `json:"request_id"`
	Status          string      `json:"status"`
	Amount          json.Number `json:"amount"`
	Currency        string      `json:"currency"`
	MerchantOrderID string      `json:"merchant_order_id"`
	ClientSecret    string      `json:"client_secret"`
}
type refund struct {
	ID              string      `json:"id"`
	RequestID       string      `json:"request_id"`
	PaymentIntentID string      `json:"payment_intent_id"`
	Status          string      `json:"status"`
	Amount          json.Number `json:"amount"`
	Currency        string      `json:"currency"`
}
