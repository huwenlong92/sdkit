package logger

import (
	"context"

	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracking"

	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	TrackIDKey   = "track_id"
	TraceIDKey   = "trace_id"
	SpanIDKey    = "span_id"
	RequestIDKey = "request_id"
	TaskIDKey    = "task_id"
	QueueKey     = "queue"
	TypeKey      = "type"
	RunIDKey     = "run_id"
	JobIDKey     = "job_id"
)

type contextKey string

var contextKeys = map[string]contextKey{
	TrackIDKey:   contextKey(TrackIDKey),
	TraceIDKey:   contextKey(TraceIDKey),
	SpanIDKey:    contextKey(SpanIDKey),
	RequestIDKey: contextKey(RequestIDKey),
	TaskIDKey:    contextKey(TaskIDKey),
	QueueKey:     contextKey(QueueKey),
	TypeKey:      contextKey(TypeKey),
	RunIDKey:     contextKey(RunIDKey),
	JobIDKey:     contextKey(JobIDKey),
}

// WithField stores a standard logger field in context.
func WithField(ctx context.Context, key, value string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if key == "" || value == "" {
		return ctx
	}
	if typedKey, ok := contextKeys[key]; ok {
		return context.WithValue(ctx, typedKey, value)
	}
	return ctx
}

// Field reads a standard logger field from context.
func Field(ctx context.Context, key string) string {
	if ctx == nil || key == "" {
		return ""
	}
	if typedKey, ok := contextKeys[key]; ok {
		if value, ok := ctx.Value(typedKey).(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func ContextFields(ctx context.Context) []zap.Field {
	if ctx == nil {
		return nil
	}

	fields := make([]zap.Field, 0, 9)
	if trackID := tracking.TrackID(ctx); trackID != "" {
		fields = append(fields, zap.String(TrackIDKey, trackID))
	} else {
		fields = appendStringContextField(ctx, fields, TrackIDKey)
	}
	hasTraceID := false
	if traceID := Field(ctx, TraceIDKey); traceID != "" {
		fields = append(fields, zap.String(TraceIDKey, traceID))
		hasTraceID = true
	}
	hasSpanID := false
	if spanID := Field(ctx, SpanIDKey); spanID != "" {
		fields = append(fields, zap.String(SpanIDKey, spanID))
		hasSpanID = true
	}
	if spanContext := oteltrace.SpanContextFromContext(ctx); spanContext.IsValid() {
		if spanContext.HasTraceID() && !hasTraceID {
			fields = append(fields, zap.String(TraceIDKey, spanContext.TraceID().String()))
		}
		if spanContext.HasSpanID() && !hasSpanID {
			fields = append(fields, zap.String(SpanIDKey, spanContext.SpanID().String()))
		}
	}
	if requestID := requestid.FromContext(ctx); requestID != "" {
		fields = append(fields, zap.String(RequestIDKey, requestID))
	} else {
		fields = appendStringContextField(ctx, fields, RequestIDKey)
	}
	fields = appendStringContextField(ctx, fields, TaskIDKey)
	fields = appendStringContextField(ctx, fields, QueueKey)
	fields = appendStringContextField(ctx, fields, TypeKey)
	fields = appendStringContextField(ctx, fields, RunIDKey)
	fields = appendStringContextField(ctx, fields, JobIDKey)
	return fields
}

func appendStringContextField(ctx context.Context, fields []zap.Field, key string) []zap.Field {
	if value := Field(ctx, key); value != "" {
		return append(fields, zap.String(key, value))
	}
	return fields
}

func WithContext(ctx context.Context, log *zap.Logger) *zap.Logger {
	if log == nil {
		log = zap.NewNop()
	}
	return log.With(ContextFields(ctx)...)
}

func Ctx(ctx context.Context) *zap.Logger {
	return WithContext(ctx, L)
}
