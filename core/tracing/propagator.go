package tracing

import (
	"context"
	"net/http"

	"github.com/huwenlong92/sdkit/core/tracecontext"
)

func InjectHTTPHeader(ctx context.Context, header http.Header) {
	tracecontext.InjectHTTPHeader(ctx, header)
}

func ExtractHTTPHeader(ctx context.Context, header http.Header) context.Context {
	return tracecontext.ExtractHTTPHeader(ctx, header)
}
