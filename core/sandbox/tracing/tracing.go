package tracing

import (
	"context"

	coretracing "github.com/huwenlong92/sdkit/core/tracing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type runRequest interface {
	GetSubmissionID() string
	GetImage() string
	GetTimeoutSeconds() float64
	GetMemoryBytes() int64
}

type result interface {
	GetContainerID() string
	GetExitCode() int
	GetTimedOut() bool
	GetMemoryUsed() uint64
	GetCPUUsed() float64
}

const tracerName = "sdkitgo/core/sandbox"

func StartRun(ctx context.Context, req interface{}) (context.Context, oteltrace.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	attrs := runRequestAttrs(req)
	ctx, span := otel.Tracer(tracerName).Start(ctx, "sandbox.run", oteltrace.WithAttributes(attrs...))
	coretracing.SetSpanCorrelationAttributes(ctx, span)
	return ctx, span
}

func StartStep(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, oteltrace.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	if name == "" {
		name = "sandbox.step"
	}
	ctx, span := otel.Tracer(tracerName).Start(ctx, name, oteltrace.WithAttributes(attrs...))
	coretracing.SetSpanCorrelationAttributes(ctx, span)
	return ctx, span
}

func SetContainerID(span oteltrace.Span, containerID string) {
	if span == nil || containerID == "" {
		return
	}
	span.SetAttributes(attribute.String("container.id", containerID))
}

func SetRunRequest(span oteltrace.Span, req interface{}) {
	if span == nil {
		return
	}
	if attrs := runRequestAttrs(req); len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
}

func SetRunResult(span oteltrace.Span, res interface{}) {
	if span == nil || res == nil {
		return
	}
	r, ok := res.(result)
	if !ok {
		return
	}
	SetContainerID(span, r.GetContainerID())
	span.SetAttributes(
		attribute.Int("exit.code", r.GetExitCode()),
		attribute.Bool("timed_out", r.GetTimedOut()),
		attribute.Int64("memory.used", int64(r.GetMemoryUsed())),
		attribute.Float64("cpu.used", r.GetCPUUsed()),
	)
}

func runRequestAttrs(req interface{}) []attribute.KeyValue {
	r, ok := req.(runRequest)
	if !ok || r == nil {
		return nil
	}
	return []attribute.KeyValue{
		attribute.String("submission.id", r.GetSubmissionID()),
		attribute.String("image", r.GetImage()),
		attribute.Float64("timeout", r.GetTimeoutSeconds()),
		attribute.Int64("memory.limit", r.GetMemoryBytes()),
	}
}

func RecordError(span oteltrace.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
