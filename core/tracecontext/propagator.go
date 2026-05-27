package tracecontext

import (
	"context"
	"net/http"
)

type HeaderCarrier http.Header

func (c HeaderCarrier) Get(key string) string {
	return http.Header(c).Get(key)
}

func (c HeaderCarrier) Set(key string, value string) {
	if key == "" || value == "" {
		return
	}
	http.Header(c).Set(key, value)
}

func (c HeaderCarrier) Keys() []string {
	header := http.Header(c)
	keys := make([]string, 0, len(header))
	for key := range header {
		keys = append(keys, key)
	}
	return keys
}

func InjectHTTPHeader(ctx context.Context, header http.Header) {
	if header == nil {
		return
	}
	InjectCarrier(ctx, HeaderCarrier(header))
}

func ExtractHTTPHeader(ctx context.Context, header http.Header) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if header == nil {
		return ctx
	}
	return ExtractCarrier(ctx, HeaderCarrier(header))
}
