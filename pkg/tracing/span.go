package tracing

import "context"

const defaultTracerName = "sdkitgo/core/tracing"

type StatusCode string

const (
	StatusUnset StatusCode = ""
	StatusOK    StatusCode = "ok"
	StatusError StatusCode = "error"
)

type SpanKind string

const (
	SpanKindInternal SpanKind = "internal"
	SpanKindServer   SpanKind = "server"
	SpanKindProducer SpanKind = "producer"
	SpanKindConsumer SpanKind = "consumer"
)

type SpanOptions struct {
	TracerName string
	Kind       SpanKind
}

type Span interface {
	End()
	IsRecording() bool
	RecordError(error)
	SetStatus(StatusCode, string)
	SetAttributes(...Attr)
	TraceID() string
	SpanID() string
}

type noopSpan struct {
	traceID string
	spanID  string
}

func (noopSpan) End() {}

func (noopSpan) IsRecording() bool {
	return false
}

func (noopSpan) RecordError(error) {}

func (noopSpan) SetStatus(StatusCode, string) {}

func (noopSpan) SetAttributes(...Attr) {}

func (s noopSpan) TraceID() string {
	return s.traceID
}

func (s noopSpan) SpanID() string {
	return s.spanID
}

func StartSpan(ctx context.Context, name string, attrs ...Attr) (context.Context, Span) {
	return StartSpanWithOptions(ctx, name, SpanOptions{}, attrs...)
}

func StartSpanWithOptions(ctx context.Context, name string, opts SpanOptions, attrs ...Attr) (context.Context, Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	if name == "" {
		name = "span"
	}
	if opts.TracerName == "" {
		opts.TracerName = defaultTracerName
	}
	return currentProvider().StartSpan(ctx, name, opts, attrs...)
}
