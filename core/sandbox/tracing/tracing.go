package tracing

import (
	"context"

	coretracing "github.com/huwenlong92/sdkit/core/tracing"
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

func StartRun(ctx context.Context, req interface{}) (context.Context, coretracing.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	attrs := runRequestAttrs(req)
	ctx, span := coretracing.StartSpanWithOptions(ctx, "sandbox.run", coretracing.SpanOptions{TracerName: tracerName}, attrs...)
	coretracing.SetSpanCorrelationAttributes(ctx, span)
	return ctx, span
}

func StartStep(ctx context.Context, name string, attrs ...coretracing.Attr) (context.Context, coretracing.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	if name == "" {
		name = "sandbox.step"
	}
	ctx, span := coretracing.StartSpanWithOptions(ctx, name, coretracing.SpanOptions{TracerName: tracerName}, attrs...)
	coretracing.SetSpanCorrelationAttributes(ctx, span)
	return ctx, span
}

func SetContainerID(span coretracing.Span, containerID string) {
	if span == nil || containerID == "" {
		return
	}
	span.SetAttributes(coretracing.String("container.id", containerID))
}

func SetRunRequest(span coretracing.Span, req interface{}) {
	if span == nil {
		return
	}
	if attrs := runRequestAttrs(req); len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
}

func SetRunResult(span coretracing.Span, res interface{}) {
	if span == nil || res == nil {
		return
	}
	r, ok := res.(result)
	if !ok {
		return
	}
	SetContainerID(span, r.GetContainerID())
	span.SetAttributes(
		coretracing.Int("exit.code", r.GetExitCode()),
		coretracing.Bool("timed_out", r.GetTimedOut()),
		coretracing.Int64("memory.used", int64(r.GetMemoryUsed())),
		coretracing.Float64("cpu.used", r.GetCPUUsed()),
	)
}

func runRequestAttrs(req interface{}) []coretracing.Attr {
	r, ok := req.(runRequest)
	if !ok || r == nil {
		return nil
	}
	return []coretracing.Attr{
		coretracing.String("submission.id", r.GetSubmissionID()),
		coretracing.String("image", r.GetImage()),
		coretracing.Float64("timeout", r.GetTimeoutSeconds()),
		coretracing.Int64("memory.limit", r.GetMemoryBytes()),
	}
}

func RecordError(span coretracing.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(coretracing.StatusError, err.Error())
}
