package redis

import (
	"context"

	coreredis "github.com/huwenlong92/sdkit/core/redis"
	"github.com/huwenlong92/sdkit/core/runtime"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func New(cfg Config, log *zap.Logger) *RuntimeClient {
	return coreredis.New(cfg, log)
}

func Init(ctx context.Context, cfg Config, log *zap.Logger) error {
	return coreredis.Init(ctx, cfg, log)
}

func Close() error {
	return coreredis.Close()
}

func Raw() *goredis.Client {
	return coreredis.Raw()
}

func Client(ctxs ...context.Context) *goredis.Client {
	return coreredis.Client(ctxs...)
}

func ClientFrom(app *runtime.App) *goredis.Client {
	return coreredis.ClientFrom(app)
}

func Health(ctx context.Context) error {
	return coreredis.Health(ctx)
}
