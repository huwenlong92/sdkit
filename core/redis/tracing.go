package redis

import (
	"context"
	"errors"
	"strings"

	"github.com/huwenlong92/sdkit/core/tracing"

	goredis "github.com/redis/go-redis/v9"
)

func startRedisCommandSpan(ctx context.Context, name string) (context.Context, tracing.Span) {
	return startRedisSpan(ctx, "redis."+name,
		tracing.String("db.system", "redis"),
		tracing.String("db.operation", name),
	)
}

func startRedisPipelineSpan(ctx context.Context, names []string) (context.Context, tracing.Span) {
	return startRedisSpan(ctx, "redis.pipeline",
		tracing.String("db.system", "redis"),
		tracing.String("db.operation", "pipeline"),
		tracing.Int("redis.pipeline.length", len(names)),
		tracing.String("redis.pipeline.commands", strings.Join(names, ",")),
	)
}

func startRedisSpan(ctx context.Context, name string, attrs ...tracing.Attr) (context.Context, tracing.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := tracing.StartSpanWithOptions(ctx, name, tracing.SpanOptions{TracerName: "sdkitgo/core/redis"}, attrs...)
	tracing.SetSpanCorrelationAttributes(ctx, span)
	return ctx, span
}

func recordRedisError(span tracing.Span, err error) {
	if err == nil || errors.Is(err, goredis.Nil) {
		return
	}
	span.RecordError(err)
	span.SetStatus(tracing.StatusError, err.Error())
}
