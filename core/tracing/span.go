package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const tracerName = "sdkitgo/core/tracing"

func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, oteltrace.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	if name == "" {
		name = "span"
	}
	opts := make([]oteltrace.SpanStartOption, 0, 1)
	if len(attrs) > 0 {
		opts = append(opts, oteltrace.WithAttributes(attrs...))
	}
	return otel.Tracer(tracerName).Start(ctx, name, opts...)
}
