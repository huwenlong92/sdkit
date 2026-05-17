package crontab

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/tracking"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func startExecuteSpan(ctx context.Context, entry Entry, templateName string, cron string, allowOverlap bool, timeout time.Duration) (context.Context, oteltrace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String("entry.id", entry.ID),
		attribute.String("entry_id", entry.ID),
		attribute.String("template.name", templateName),
		attribute.String("template", templateName),
		attribute.String("cron", cron),
		attribute.Bool("allow_overlap", allowOverlap),
		attribute.String("timeout", timeout.String()),
	}
	if trackID := tracking.TrackID(ctx); trackID != "" {
		attrs = append(attrs, attribute.String("track_id", trackID))
	}
	return otel.Tracer("sdkitgo/core/crontab").Start(ctx, "crontab.execute",
		oteltrace.WithSpanKind(oteltrace.SpanKindInternal),
		oteltrace.WithAttributes(attrs...),
	)
}

func setExecuteSpanTemplate(span oteltrace.Span, templateName string, cron string, allowOverlap bool, timeout time.Duration) {
	if !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.String("template.name", templateName),
		attribute.String("template", templateName),
		attribute.String("cron", cron),
		attribute.Bool("allow_overlap", allowOverlap),
		attribute.String("timeout", timeout.String()),
	)
}

func finishRunSpan(span oteltrace.Span, status Status, err error, duration time.Duration) {
	if !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.String("crontab.status", string(status)),
		attribute.Bool("success", status == StatusSuccess),
		attribute.String("duration", duration.String()),
	)
	switch status {
	case StatusSuccess, StatusSkipped, StatusLocked, StatusDisabled, StatusTemplateDisabled:
		span.SetStatus(codes.Ok, string(status))
	default:
		span.SetStatus(codes.Error, string(status))
	}
	if err != nil {
		span.RecordError(err)
	}
}
