package ordersapi

import (
	"context"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/debuglog"
)

type debugEvent struct {
	Stage     debuglog.Stage
	Operation string
	Request   any
	Response  any
	Err       error
}

func (c *Client) debug(ctx context.Context, event debugEvent) {
	if c.debugLogger == nil {
		return
	}
	out := debuglog.Event{
		Component: "paypal_ordersapi",
		Stage:     event.Stage,
		Operation: event.Operation,
		Provider:  payment.ProviderPayPal,
		Request:   event.Request,
		Response:  event.Response,
		Err:       event.Err,
	}
	if c.debugPayloadMode != debuglog.PayloadFull {
		out.Request = nil
		out.Response = nil
	}
	c.debugLogger.DebugPayment(ctx, out)
}
