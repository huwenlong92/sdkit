package channelrouter

import (
	"context"
	"fmt"

	"github.com/huwenlong92/sdkit/core/payment"
)

func NewAdapter(cfg Config) (*Adapter, error) {
	if cfg.Provider == "" {
		return nil, fmt.Errorf("%w: channelrouter provider is required", payment.ErrUnsupportedProvider)
	}
	resolver := cfg.Resolver
	var static *StaticResolver
	var err error
	if resolver == nil {
		static, err = NewStaticResolver(cfg.Routes)
		if err != nil {
			return nil, err
		}
		resolver = static
	}
	caps := capabilities(cfg.Provider, cfg.Routes)
	if caps.Provider == "" {
		caps.Provider = cfg.Provider
	}
	return &Adapter{
		provider:         cfg.Provider,
		resolver:         resolver,
		debugLogger:      cfg.DebugLogger,
		debugPayloadMode: cfg.DebugPayloadMode,
		capabilities:     caps,
	}, nil
}

func (a *Adapter) Name() payment.Provider {
	return a.provider
}

func (a *Adapter) Capabilities() payment.Capabilities {
	return a.capabilities
}

func (a *Adapter) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	route, err := a.resolve(ctx, ResolveRequest{Provider: req.Provider, Channel: req.Channel, MerchantKey: req.MerchantKey, Operation: OperationCreate})
	if err != nil {
		return nil, err
	}
	req.MerchantKey = route.MerchantKey
	a.debug(ctx, a.withRequest(createDebugEvent(DebugStageOperationStart, OperationCreate, req, route, nil), req))
	resp, err := route.Adapter.CreatePayment(ctx, req)
	if err != nil {
		a.debug(ctx, a.withRequest(createDebugEvent(DebugStageOperationFailed, OperationCreate, req, route, err), req))
		return nil, err
	}
	if resp != nil {
		fillCreate(resp, route)
	}
	a.debug(ctx, a.withRequestResponse(createResponseDebugEvent(DebugStageOperationDone, OperationCreate, req, resp, route, nil), req, resp))
	return resp, nil
}

func (a *Adapter) QueryPayment(ctx context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	route, err := a.resolve(ctx, ResolveRequest{Provider: req.Provider, Channel: req.Channel, MerchantKey: req.MerchantKey, Operation: OperationQuery})
	if err != nil {
		return nil, err
	}
	req.MerchantKey = route.MerchantKey
	a.debug(ctx, a.withRequest(queryDebugEvent(DebugStageOperationStart, OperationQuery, req, route, nil), req))
	resp, err := route.Adapter.QueryPayment(ctx, req)
	if err != nil {
		a.debug(ctx, a.withRequest(queryDebugEvent(DebugStageOperationFailed, OperationQuery, req, route, err), req))
		return nil, err
	}
	if resp != nil {
		fillQuery(resp, route)
	}
	a.debug(ctx, a.withRequestResponse(queryResponseDebugEvent(DebugStageOperationDone, OperationQuery, req, resp, route, nil), req, resp))
	return resp, nil
}

func (a *Adapter) ClosePayment(ctx context.Context, req payment.ClosePaymentRequest) error {
	route, err := a.resolve(ctx, ResolveRequest{Provider: req.Provider, Channel: req.Channel, MerchantKey: req.MerchantKey, Operation: OperationClose})
	if err != nil {
		return err
	}
	req.MerchantKey = route.MerchantKey
	a.debug(ctx, a.withRequest(closeDebugEvent(DebugStageOperationStart, OperationClose, req, route, nil), req))
	err = route.Adapter.ClosePayment(ctx, req)
	if err != nil {
		a.debug(ctx, a.withRequest(closeDebugEvent(DebugStageOperationFailed, OperationClose, req, route, err), req))
		return err
	}
	a.debug(ctx, a.withRequest(closeDebugEvent(DebugStageOperationDone, OperationClose, req, route, nil), req))
	return nil
}

func (a *Adapter) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	route, err := a.resolve(ctx, ResolveRequest{Provider: req.Provider, Channel: req.Channel, MerchantKey: req.MerchantKey, Operation: OperationRefund})
	if err != nil {
		return nil, err
	}
	req.MerchantKey = route.MerchantKey
	a.debug(ctx, a.withRequest(refundDebugEvent(DebugStageOperationStart, OperationRefund, req, route, nil), req))
	resp, err := route.Adapter.Refund(ctx, req)
	if err != nil {
		a.debug(ctx, a.withRequest(refundDebugEvent(DebugStageOperationFailed, OperationRefund, req, route, err), req))
		return nil, err
	}
	if resp != nil {
		fillRefund(resp, route)
	}
	a.debug(ctx, a.withRequestResponse(refundResponseDebugEvent(DebugStageOperationDone, OperationRefund, req, resp, route, nil), req, resp))
	return resp, nil
}

func (a *Adapter) QueryRefund(ctx context.Context, req payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	route, err := a.resolve(ctx, ResolveRequest{Provider: req.Provider, Channel: req.Channel, MerchantKey: req.MerchantKey, Operation: OperationQueryRefund})
	if err != nil {
		return nil, err
	}
	req.MerchantKey = route.MerchantKey
	a.debug(ctx, a.withRequest(queryRefundDebugEvent(DebugStageOperationStart, OperationQueryRefund, req, route, nil), req))
	resp, err := route.Adapter.QueryRefund(ctx, req)
	if err != nil {
		a.debug(ctx, a.withRequest(queryRefundDebugEvent(DebugStageOperationFailed, OperationQueryRefund, req, route, err), req))
		return nil, err
	}
	if resp != nil {
		fillQueryRefund(resp, route)
	}
	a.debug(ctx, a.withRequestResponse(queryRefundResponseDebugEvent(DebugStageOperationDone, OperationQueryRefund, req, resp, route, nil), req, resp))
	return resp, nil
}

func (a *Adapter) ParseNotify(ctx context.Context, req payment.NotifyRequest) (*payment.NotifyResult, error) {
	merchantKey := ""
	if req.Query != nil {
		merchantKey = first(req.Query["merchant_key"])
	}
	route, err := a.resolve(ctx, ResolveRequest{Provider: req.Provider, Channel: req.Channel, MerchantKey: merchantKey, Operation: OperationNotify})
	if err != nil {
		return nil, err
	}
	return route.Adapter.ParseNotify(ctx, req)
}

func (a *Adapter) resolve(ctx context.Context, req ResolveRequest) (*ResolvedRoute, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	if req.Provider == "" {
		req.Provider = a.provider
	}
	if req.Provider != a.provider {
		return nil, fmt.Errorf("%w: channelrouter provider %s does not match %s", payment.ErrUnsupportedProvider, req.Provider, a.provider)
	}
	a.debug(ctx, DebugEvent{
		Stage:                DebugStageResolveStart,
		Operation:            req.Operation,
		Provider:             req.Provider,
		Channel:              req.Channel,
		RequestedMerchantKey: req.MerchantKey,
	})
	route, err := a.resolver.ResolvePaymentChannel(ctx, req)
	if err != nil {
		a.debug(ctx, DebugEvent{
			Stage:                DebugStageResolveFailed,
			Operation:            req.Operation,
			Provider:             req.Provider,
			Channel:              req.Channel,
			RequestedMerchantKey: req.MerchantKey,
			Err:                  err,
		})
		return nil, err
	}
	if route == nil || route.Adapter == nil {
		err := fmt.Errorf("%w: payment channel route not found", payment.ErrAdapterNotFound)
		a.debug(ctx, DebugEvent{
			Stage:                DebugStageResolveFailed,
			Operation:            req.Operation,
			Provider:             req.Provider,
			Channel:              req.Channel,
			RequestedMerchantKey: req.MerchantKey,
			Err:                  err,
		})
		return nil, err
	}
	if route.Provider == "" {
		route.Provider = a.provider
	}
	if route.Channel == "" {
		route.Channel = req.Channel
	}
	if route.MerchantKey == "" {
		return nil, fmt.Errorf("%w: resolved merchant key is required", payment.ErrInvalidRequest)
	}
	if route.Provider != a.provider {
		return nil, fmt.Errorf("%w: resolved provider %s does not match %s", payment.ErrUnsupportedProvider, route.Provider, a.provider)
	}
	a.debug(ctx, DebugEvent{
		Stage:                DebugStageResolveSucceeded,
		Operation:            req.Operation,
		Provider:             route.Provider,
		Channel:              route.Channel,
		RequestedMerchantKey: req.MerchantKey,
		ResolvedMerchantKey:  route.MerchantKey,
	})
	return route, nil
}

func (a *Adapter) debug(ctx context.Context, event DebugEvent) {
	if a.debugLogger != nil {
		a.debugLogger.DebugPaymentChannel(ctx, event)
	}
}

func (a *Adapter) withRequest(event DebugEvent, req any) DebugEvent {
	if a.debugPayloadMode == DebugPayloadFull {
		event.Request = req
	}
	return event
}

func (a *Adapter) withRequestResponse(event DebugEvent, req any, resp any) DebugEvent {
	if a.debugPayloadMode == DebugPayloadFull {
		event.Request = req
		event.Response = resp
	}
	return event
}
