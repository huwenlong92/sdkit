package payment

import (
	"fmt"
	"strings"
	"time"
)

func validateCreatePaymentRequest(req CreatePaymentRequest, now time.Time) error {
	if req.Provider == "" {
		return fmt.Errorf("%w: provider is required", ErrInvalidRequest)
	}
	if req.Channel == "" {
		return fmt.Errorf("%w: channel is required", ErrInvalidRequest)
	}
	if !hasPaymentReference(req.PaymentID, req.OutTradeNo, "", "") {
		return fmt.Errorf("%w: payment_id or out_trade_no is required", ErrInvalidRequest)
	}
	if req.ExpireAt != nil && !req.ExpireAt.After(now) {
		return fmt.Errorf("%w: expire_at must be in the future", ErrInvalidRequest)
	}
	return nil
}

func validateQueryPaymentRequest(req QueryPaymentRequest) error {
	if req.Provider == "" {
		return fmt.Errorf("%w: provider is required", ErrInvalidRequest)
	}
	if req.Channel == "" {
		return fmt.Errorf("%w: channel is required", ErrInvalidRequest)
	}
	if !hasPaymentReference(req.PaymentID, req.OutTradeNo, req.ProviderTradeID, req.OrderID) {
		return fmt.Errorf("%w: payment reference is required", ErrInvalidRequest)
	}
	return nil
}

func validateClosePaymentRequest(req ClosePaymentRequest) error {
	if req.Provider == "" {
		return fmt.Errorf("%w: provider is required", ErrInvalidRequest)
	}
	if req.Channel == "" {
		return fmt.Errorf("%w: channel is required", ErrInvalidRequest)
	}
	if !hasPaymentReference(req.PaymentID, req.OutTradeNo, req.ProviderTradeID, req.OrderID) {
		return fmt.Errorf("%w: payment reference is required", ErrInvalidRequest)
	}
	return nil
}

func validateRefundRequest(req RefundRequest) error {
	if req.Provider == "" {
		return fmt.Errorf("%w: provider is required", ErrInvalidRequest)
	}
	if req.Channel == "" {
		return fmt.Errorf("%w: channel is required", ErrInvalidRequest)
	}
	if !hasPaymentReference(req.PaymentID, req.ProviderTradeID, req.OutTradeNo, req.OrderID) {
		return fmt.Errorf("%w: payment reference is required", ErrInvalidRequest)
	}
	return nil
}

func validateQueryRefundRequest(req QueryRefundRequest) error {
	if req.Provider == "" {
		return fmt.Errorf("%w: provider is required", ErrInvalidRequest)
	}
	if req.Channel == "" {
		return fmt.Errorf("%w: channel is required", ErrInvalidRequest)
	}
	if strings.TrimSpace(req.RefundID) == "" &&
		strings.TrimSpace(req.OutRefundNo) == "" &&
		strings.TrimSpace(req.ProviderRefundID) == "" {
		return fmt.Errorf("%w: refund reference is required", ErrInvalidRequest)
	}
	return nil
}

func validateNotifyRequest(req NotifyRequest) error {
	if req.Provider == "" {
		return fmt.Errorf("%w: provider is required", ErrInvalidRequest)
	}
	if req.Channel == "" {
		return fmt.Errorf("%w: channel is required", ErrInvalidRequest)
	}
	return nil
}

func normalizeAndValidateAction(action PaymentAction) (PaymentAction, error) {
	if action.Type == "" {
		action.Type = ActionNone
	}
	switch action.Type {
	case ActionNone:
		return action, nil
	case ActionRedirectURL:
		if strings.TrimSpace(action.URL) == "" {
			return action, fmt.Errorf("%w: redirect_url requires url", ErrInvalidAction)
		}
	case ActionHTMLForm:
		if strings.TrimSpace(action.HTML) != "" {
			return action, nil
		}
		if strings.TrimSpace(action.URL) == "" {
			return action, fmt.Errorf("%w: html_form requires html or url", ErrInvalidAction)
		}
		if len(action.Fields) == 0 {
			return action, fmt.Errorf("%w: html_form requires fields when html is empty", ErrInvalidAction)
		}
		if strings.TrimSpace(action.Method) == "" {
			action.Method = "POST"
		}
	case ActionQRCode:
		if strings.TrimSpace(action.URL) == "" && strings.TrimSpace(action.Token) == "" {
			return action, fmt.Errorf("%w: qr_code requires url or token", ErrInvalidAction)
		}
	case ActionSDKParams:
		if len(action.Params) == 0 {
			return action, fmt.Errorf("%w: sdk_params requires params", ErrInvalidAction)
		}
	case ActionClientToken:
		if strings.TrimSpace(action.Token) == "" {
			return action, fmt.Errorf("%w: client_token requires token", ErrInvalidAction)
		}
	default:
		return action, fmt.Errorf("%w: unknown action type %s", ErrInvalidAction, action.Type)
	}
	return action, nil
}

func hasPaymentReference(paymentID, outTradeNo, providerTradeID, orderID string) bool {
	return strings.TrimSpace(paymentID) != "" ||
		strings.TrimSpace(outTradeNo) != "" ||
		strings.TrimSpace(providerTradeID) != "" ||
		strings.TrimSpace(orderID) != ""
}
