//go:build sdkit_payment_stripe

package stripego

import (
	"context"

	"github.com/huwenlong92/sdkit/pkg/payment/debuglog"
	official "github.com/stripe/stripe-go/v85"
)

const (
	ExtraCancelURLKey          = "cancel_url"
	ExtraCustomerIDKey         = "customer_id"
	ExtraReceiptEmailKey       = "receipt_email"
	ExtraPaymentMethodTypesKey = "payment_method_types"
	ExtraChargeIDKey           = "charge_id"
	ExtraPaymentIntentIDKey    = "payment_intent_id"
	ExtraCheckoutSessionIDKey  = "checkout_session_id"
)

type CheckoutService interface {
	CreateCheckoutSession(ctx context.Context, params *official.CheckoutSessionCreateParams) (*official.CheckoutSession, error)
	RetrieveCheckoutSession(ctx context.Context, id string, params *official.CheckoutSessionRetrieveParams) (*official.CheckoutSession, error)
	ExpireCheckoutSession(ctx context.Context, id string, params *official.CheckoutSessionExpireParams) (*official.CheckoutSession, error)
}

type PaymentIntentService interface {
	CreatePaymentIntent(ctx context.Context, params *official.PaymentIntentCreateParams) (*official.PaymentIntent, error)
	RetrievePaymentIntent(ctx context.Context, id string, params *official.PaymentIntentRetrieveParams) (*official.PaymentIntent, error)
	CancelPaymentIntent(ctx context.Context, id string, params *official.PaymentIntentCancelParams) (*official.PaymentIntent, error)
}

type RefundService interface {
	CreateRefund(ctx context.Context, params *official.RefundCreateParams) (*official.Refund, error)
	RetrieveRefund(ctx context.Context, id string, params *official.RefundRetrieveParams) (*official.Refund, error)
}

type Config struct {
	APIKey     string
	SuccessURL string
	CancelURL  string

	OfficialClient       *official.Client
	CheckoutService      CheckoutService
	PaymentIntentService PaymentIntentService
	RefundService        RefundService
	DebugLogger          debuglog.Logger
	DebugPayloadMode     debuglog.PayloadMode
}

type Client struct {
	successURL           string
	cancelURL            string
	checkoutService      CheckoutService
	paymentIntentService PaymentIntentService
	refundService        RefundService
	debugLogger          debuglog.Logger
	debugPayloadMode     debuglog.PayloadMode
}

type officialServices struct {
	client *official.Client
}

func (s officialServices) CreateCheckoutSession(ctx context.Context, params *official.CheckoutSessionCreateParams) (*official.CheckoutSession, error) {
	return s.client.V1CheckoutSessions.Create(ctx, params)
}

func (s officialServices) RetrieveCheckoutSession(ctx context.Context, id string, params *official.CheckoutSessionRetrieveParams) (*official.CheckoutSession, error) {
	return s.client.V1CheckoutSessions.Retrieve(ctx, id, params)
}

func (s officialServices) ExpireCheckoutSession(ctx context.Context, id string, params *official.CheckoutSessionExpireParams) (*official.CheckoutSession, error) {
	return s.client.V1CheckoutSessions.Expire(ctx, id, params)
}

func (s officialServices) CreatePaymentIntent(ctx context.Context, params *official.PaymentIntentCreateParams) (*official.PaymentIntent, error) {
	return s.client.V1PaymentIntents.Create(ctx, params)
}

func (s officialServices) RetrievePaymentIntent(ctx context.Context, id string, params *official.PaymentIntentRetrieveParams) (*official.PaymentIntent, error) {
	return s.client.V1PaymentIntents.Retrieve(ctx, id, params)
}

func (s officialServices) CancelPaymentIntent(ctx context.Context, id string, params *official.PaymentIntentCancelParams) (*official.PaymentIntent, error) {
	return s.client.V1PaymentIntents.Cancel(ctx, id, params)
}

func (s officialServices) CreateRefund(ctx context.Context, params *official.RefundCreateParams) (*official.Refund, error) {
	return s.client.V1Refunds.Create(ctx, params)
}

func (s officialServices) RetrieveRefund(ctx context.Context, id string, params *official.RefundRetrieveParams) (*official.Refund, error) {
	return s.client.V1Refunds.Retrieve(ctx, id, params)
}
