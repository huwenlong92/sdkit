package crontab

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/tracing"
)

func startExecuteSpan(ctx context.Context, entry Entry, templateName string, cron string, allowOverlap bool, timeout time.Duration) (context.Context, tracing.Span) {
	attrs := []tracing.Attr{
		tracing.String("entry.id", entry.ID),
		tracing.String("entry_id", entry.ID),
		tracing.String("template.name", templateName),
		tracing.String("template", templateName),
		tracing.String("cron", cron),
		tracing.Bool("allow_overlap", allowOverlap),
		tracing.String("timeout", timeout.String()),
	}
	ctx, span := tracing.StartSpanWithOptions(ctx, "crontab.execute", tracing.SpanOptions{
		TracerName: "sdkitgo/core/crontab",
		Kind:       tracing.SpanKindInternal,
	}, attrs...)
	tracing.SetSpanCorrelationAttributes(ctx, span)
	return ctx, span
}

func setExecuteSpanTemplate(span tracing.Span, templateName string, cron string, allowOverlap bool, timeout time.Duration) {
	if !span.IsRecording() {
		return
	}
	span.SetAttributes(
		tracing.String("template.name", templateName),
		tracing.String("template", templateName),
		tracing.String("cron", cron),
		tracing.Bool("allow_overlap", allowOverlap),
		tracing.String("timeout", timeout.String()),
	)
}

func finishRunSpan(span tracing.Span, status Status, err error, duration time.Duration) {
	if !span.IsRecording() {
		return
	}
	span.SetAttributes(
		tracing.String("crontab.status", string(status)),
		tracing.Bool("success", status == StatusSuccess),
		tracing.String("duration", duration.String()),
	)
	switch status {
	case StatusSuccess, StatusSkipped, StatusLocked, StatusDisabled, StatusTemplateDisabled:
		span.SetStatus(tracing.StatusOK, string(status))
	default:
		span.SetStatus(tracing.StatusError, string(status))
	}
	if err != nil {
		span.RecordError(err)
	}
}
