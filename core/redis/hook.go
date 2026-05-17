package redis

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/tracing"

	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Hook struct {
	Log *zap.Logger
}

func NewHook(log *zap.Logger) Hook {
	if log == nil {
		log = zap.NewNop()
	}
	return Hook{Log: log}
}

func (h Hook) DialHook(next goredis.DialHook) goredis.DialHook {
	return next
}

func (h Hook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		ctx, span := startRedisSpan(ctx, "redis."+cmd.Name(),
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", cmd.Name()),
		)
		defer span.End()

		start := time.Now()
		err := next(ctx, cmd)
		recordRedisError(span, err)
		h.log(ctx, "redis command", err, zap.String("cmd", cmd.Name()), zap.Duration("latency", time.Since(start)))
		return err
	}
}

func (h Hook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		names := make([]string, 0, len(cmds))
		for _, cmd := range cmds {
			names = append(names, cmd.Name())
		}
		ctx, span := startRedisSpan(ctx, "redis.pipeline",
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "pipeline"),
			attribute.Int("redis.pipeline.length", len(cmds)),
			attribute.String("redis.pipeline.commands", strings.Join(names, ",")),
		)
		defer span.End()

		start := time.Now()
		err := next(ctx, cmds)
		recordRedisError(span, err)

		h.log(ctx, "redis pipeline", err,
			zap.Int("cmd_count", len(cmds)),
			zap.Strings("cmds", names),
			zap.Duration("latency", time.Since(start)),
		)
		return err
	}
}

func startRedisSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, oteltrace.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !oteltrace.SpanContextFromContext(ctx).IsValid() {
		return ctx, oteltrace.SpanFromContext(ctx)
	}
	ctx, span := otel.Tracer("sdkitgo/core/redis").Start(ctx, name, oteltrace.WithAttributes(attrs...))
	setRedisCorrelationAttributes(ctx, span)
	return ctx, span
}

func setRedisCorrelationAttributes(ctx context.Context, span oteltrace.Span) {
	tracing.SetSpanCorrelationAttributes(ctx, span)
}

func recordRedisError(span oteltrace.Span, err error) {
	if err == nil || errors.Is(err, goredis.Nil) {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

func (h Hook) log(ctx context.Context, msg string, err error, fields ...zap.Field) {
	if h.Log == nil {
		return
	}

	fields = appendContextFields(ctx, fields)
	if err != nil && !errors.Is(err, goredis.Nil) {
		fields = append(fields, zap.Error(err))
		h.Log.Warn(msg, fields...)
		return
	}
	h.Log.Debug(msg, fields...)
}

func appendContextFields(ctx context.Context, fields []zap.Field) []zap.Field {
	if ctx == nil {
		return fields
	}
	return append(fields, logger.ContextFields(ctx)...)
}
