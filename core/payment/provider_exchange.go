package payment

import "errors"

// ProviderExchange is the credential-free HTTP evidence returned by a
// provider adapter. Request and response bytes are kept out of ordinary JSON
// projections and application logs.
type ProviderExchange struct {
	RequestBody  []byte              `json:"-"`
	ResponseBody []byte              `json:"-"`
	StatusCode   int                 `json:"-"`
	Headers      map[string][]string `json:"-"`
}

type providerExchangeCarrier interface {
	ProviderExchange() ProviderExchange
}

type providerExchangeError struct {
	err      error
	exchange ProviderExchange
}

func (e *providerExchangeError) Error() string { return e.err.Error() }
func (e *providerExchangeError) Unwrap() error { return e.err }
func (e *providerExchangeError) ProviderExchange() ProviderExchange {
	return cloneProviderExchange(e.exchange)
}

// WithProviderExchange attaches credential-free raw request/response evidence
// to an error without changing errors.Is/errors.As behavior.
func WithProviderExchange(err error, exchange ProviderExchange) error {
	if err == nil || (len(exchange.RequestBody) == 0 && len(exchange.ResponseBody) == 0) {
		return err
	}
	return &providerExchangeError{err: err, exchange: cloneProviderExchange(exchange)}
}

// ProviderExchangeFromError extracts provider evidence from a failed call.
func ProviderExchangeFromError(err error) (ProviderExchange, bool) {
	var carrier providerExchangeCarrier
	if !errors.As(err, &carrier) {
		return ProviderExchange{}, false
	}
	exchange := carrier.ProviderExchange()
	return exchange, len(exchange.RequestBody) > 0 || len(exchange.ResponseBody) > 0
}

func cloneProviderExchange(exchange ProviderExchange) ProviderExchange {
	copy := ProviderExchange{
		RequestBody: append([]byte(nil), exchange.RequestBody...), ResponseBody: append([]byte(nil), exchange.ResponseBody...),
		StatusCode: exchange.StatusCode,
	}
	if len(exchange.Headers) > 0 {
		copy.Headers = make(map[string][]string, len(exchange.Headers))
		for key, values := range exchange.Headers {
			copy.Headers[key] = append([]string(nil), values...)
		}
	}
	return copy
}
