package tracing

import (
	"context"

	pkgtracing "github.com/huwenlong92/sdkit/pkg/tracing"
)

const tracerName = "sdkitgo/core/tracing"

type Attr = pkgtracing.Attr
type Span = pkgtracing.Span
type SpanKind = pkgtracing.SpanKind
type SpanOptions = pkgtracing.SpanOptions
type StatusCode = pkgtracing.StatusCode

const (
	SpanKindInternal = pkgtracing.SpanKindInternal
	SpanKindServer   = pkgtracing.SpanKindServer
	SpanKindProducer = pkgtracing.SpanKindProducer
	SpanKindConsumer = pkgtracing.SpanKindConsumer
	StatusUnset      = pkgtracing.StatusUnset
	StatusOK         = pkgtracing.StatusOK
	StatusError      = pkgtracing.StatusError
)

func String(key string, value string) Attr   { return pkgtracing.String(key, value) }
func Int(key string, value int) Attr         { return pkgtracing.Int(key, value) }
func Int64(key string, value int64) Attr     { return pkgtracing.Int64(key, value) }
func Float64(key string, value float64) Attr { return pkgtracing.Float64(key, value) }
func Bool(key string, value bool) Attr       { return pkgtracing.Bool(key, value) }

func StartSpan(ctx context.Context, name string, attrs ...Attr) (context.Context, Span) {
	return pkgtracing.StartSpanWithOptions(ctx, name, SpanOptions{TracerName: tracerName}, attrs...)
}

func StartSpanWithOptions(ctx context.Context, name string, opts SpanOptions, attrs ...Attr) (context.Context, Span) {
	return pkgtracing.StartSpanWithOptions(ctx, name, opts, attrs...)
}
