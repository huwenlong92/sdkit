package payment

func StatusFromEvent(event PaymentEvent) PaymentStatus {
	if event.Status != "" {
		return event.Status
	}
	switch event.Type {
	case EventPaymentCreated:
		return PaymentPending
	case EventPaymentSucceeded, EventPaymentCaptured:
		return PaymentSucceeded
	case EventPaymentFailed:
		return PaymentFailed
	case EventPaymentClosed:
		return PaymentClosed
	case EventPaymentAuthorized:
		return PaymentAuthorized
	case EventPaymentRefunding:
		return PaymentRefunding
	case EventPaymentPartRefund:
		return PaymentPartialRefunded
	case EventPaymentRefunded:
		return PaymentRefunded
	case EventRefundProcessing:
		return PaymentRefunding
	case EventRefundSucceeded:
		return PaymentRefunded
	}
	return ""
}

func CanTransitionPaymentStatus(from, to PaymentStatus) bool {
	if to == "" {
		return false
	}
	if from == "" || from == to {
		return true
	}
	if to == PaymentSucceeded {
		return from != PaymentRefunded
	}
	switch from {
	case PaymentPending:
		return to == PaymentProcessing ||
			to == PaymentRequiresAction ||
			to == PaymentAuthorized ||
			to == PaymentFailed ||
			to == PaymentClosed
	case PaymentProcessing, PaymentRequiresAction:
		return to == PaymentAuthorized ||
			to == PaymentFailed ||
			to == PaymentClosed
	case PaymentAuthorized:
		return to == PaymentFailed ||
			to == PaymentClosed
	case PaymentSucceeded:
		return to == PaymentRefunding ||
			to == PaymentPartialRefunded ||
			to == PaymentRefunded
	case PaymentRefunding:
		return to == PaymentSucceeded ||
			to == PaymentPartialRefunded ||
			to == PaymentRefunded
	case PaymentPartialRefunded:
		return to == PaymentRefunding ||
			to == PaymentRefunded
	case PaymentFailed, PaymentClosed:
		return to == PaymentAuthorized
	case PaymentRefunded:
		return false
	default:
		return false
	}
}
