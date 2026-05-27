package transport

import (
	"context"

	"github.com/huwenlong92/sdkit/core/tracecontext"
)

func Warn(ctx context.Context, log Logger, msg string, err error, fields ...any) {
	log = NormalizeLogger(log)
	values := make([]any, 0, len(fields)+4)
	values = append(values, "trace_id", tracecontext.TraceID(ctx))
	values = append(values, fields...)
	if err != nil {
		values = append(values, "err", err)
	}
	log.Warn(msg, values...)
}

func WarnTrace(log Logger, traceID string, msg string, err error, fields ...any) {
	log = NormalizeLogger(log)
	values := make([]any, 0, len(fields)+4)
	values = append(values, "trace_id", traceID)
	values = append(values, fields...)
	if err != nil {
		values = append(values, "err", err)
	}
	log.Warn(msg, values...)
}
