package redis

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/tracing"

	goredis "github.com/redis/go-redis/v9"
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
			tracing.String("db.system", "redis"),
			tracing.String("db.operation", cmd.Name()),
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
			tracing.String("db.system", "redis"),
			tracing.String("db.operation", "pipeline"),
			tracing.Int("redis.pipeline.length", len(cmds)),
			tracing.String("redis.pipeline.commands", strings.Join(names, ",")),
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

func startRedisSpan(ctx context.Context, name string, attrs ...tracing.Attr) (context.Context, tracing.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := tracing.StartSpanWithOptions(ctx, name, tracing.SpanOptions{TracerName: "sdkitgo/core/redis"}, attrs...)
	setRedisCorrelationAttributes(ctx, span)
	return ctx, span
}

func setRedisCorrelationAttributes(ctx context.Context, span tracing.Span) {
	tracing.SetSpanCorrelationAttributes(ctx, span)
}

func recordRedisError(span tracing.Span, err error) {
	if err == nil || errors.Is(err, goredis.Nil) {
		return
	}
	span.RecordError(err)
	span.SetStatus(tracing.StatusError, err.Error())
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
