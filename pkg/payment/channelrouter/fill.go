package channelrouter

import "github.com/huwenlong92/sdkit/core/payment"

func fillCreate(resp *payment.CreatePaymentResponse, route *ResolvedRoute) {
	resp.Provider = route.Provider
	resp.Channel = route.Channel
	resp.MerchantKey = route.MerchantKey
}

func fillQuery(resp *payment.QueryPaymentResponse, route *ResolvedRoute) {
	resp.Provider = route.Provider
	resp.Channel = route.Channel
	resp.MerchantKey = route.MerchantKey
}

func fillRefund(resp *payment.RefundResponse, route *ResolvedRoute) {
	resp.Provider = route.Provider
	resp.Channel = route.Channel
	resp.MerchantKey = route.MerchantKey
}

func fillQueryRefund(resp *payment.QueryRefundResponse, route *ResolvedRoute) {
	resp.Provider = route.Provider
	resp.Channel = route.Channel
	resp.MerchantKey = route.MerchantKey
}

func first(values []string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
