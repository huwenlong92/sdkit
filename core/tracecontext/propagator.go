package tracecontext

import (
	"context"
	"net/http"

	"github.com/huwenlong92/sdkit/core/tracking"

	"go.opentelemetry.io/otel/propagation"
)

func InjectHTTPHeader(ctx context.Context, header http.Header) {
	if header == nil {
		return
	}
	InjectCarrier(ctx, propagation.HeaderCarrier(header))
}

func ExtractHTTPHeader(ctx context.Context, header http.Header) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if header == nil {
		return ctx
	}
	return ExtractCarrier(ctx, propagation.HeaderCarrier(header))
}

func NewPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
		trackIDPropagator{},
	)
}

type trackIDPropagator struct{}

func (trackIDPropagator) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	if carrier == nil {
		return
	}
	if trackID := tracking.TrackID(ctx); trackID != "" {
		carrier.Set(tracking.Header, trackID)
	}
}

func (trackIDPropagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if carrier == nil {
		return ctx
	}
	if trackID := carrier.Get(tracking.Header); trackID != "" {
		return tracking.WithTrackID(ctx, trackID)
	}
	return ctx
}

func (trackIDPropagator) Fields() []string {
	return []string{tracking.Header}
}
