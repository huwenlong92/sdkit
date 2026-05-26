package channelrouter

import "github.com/huwenlong92/sdkit/core/payment"

func capabilities(provider payment.Provider, routes []Route) payment.Capabilities {
	caps := payment.Capabilities{
		Provider:              provider,
		SupportsMultiMerchant: true,
	}
	seenChannels := map[payment.Channel]bool{}
	seenCurrencies := map[string]bool{}
	seenActions := map[payment.ActionType]bool{}
	for _, route := range routes {
		if route.Provider != provider || route.Adapter == nil {
			continue
		}
		routeCaps := route.Adapter.Capabilities()
		for _, channel := range route.Channels {
			if !seenChannels[channel] {
				caps.Channels = append(caps.Channels, channel)
				seenChannels[channel] = true
			}
		}
		for _, currency := range routeCaps.SupportedCurrencies {
			currency = payment.NormalizeCurrency(currency)
			if !seenCurrencies[currency] {
				caps.SupportedCurrencies = append(caps.SupportedCurrencies, currency)
				seenCurrencies[currency] = true
			}
		}
		for _, action := range routeCaps.SupportedActions {
			if !seenActions[action] {
				caps.SupportedActions = append(caps.SupportedActions, action)
				seenActions[action] = true
			}
		}
		caps.SupportsExpireAt = caps.SupportsExpireAt || routeCaps.SupportsExpireAt
		caps.SupportsClose = caps.SupportsClose || routeCaps.SupportsClose
		caps.SupportsRefund = caps.SupportsRefund || routeCaps.SupportsRefund
		caps.SupportsQueryRefund = caps.SupportsQueryRefund || routeCaps.SupportsQueryRefund
		caps.SupportsPartialRefund = caps.SupportsPartialRefund || routeCaps.SupportsPartialRefund
		caps.SupportsQuery = caps.SupportsQuery || routeCaps.SupportsQuery
		caps.SupportsNotify = caps.SupportsNotify || routeCaps.SupportsNotify
		caps.SupportsCapture = caps.SupportsCapture || routeCaps.SupportsCapture
		caps.SupportsConfirm = caps.SupportsConfirm || routeCaps.SupportsConfirm
		caps.SupportsReconcile = caps.SupportsReconcile || routeCaps.SupportsReconcile
		caps.SupportsDispute = caps.SupportsDispute || routeCaps.SupportsDispute
		caps.SupportsSubscription = caps.SupportsSubscription || routeCaps.SupportsSubscription
	}
	return caps
}
