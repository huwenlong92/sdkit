package redis

import (
	"context"
	"errors"
	"sync"

	"github.com/huwenlong92/sdkit/core/runtime"
	"github.com/huwenlong92/sdkit/pkg/redisx"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var ErrNotInitialized = errors.New("redis not initialized")

var (
	Default *RuntimeClient
	RDB     *goredis.Client

	mu sync.Mutex
)

type Config = redisx.Config
type RuntimeClient = redisx.Client

func New(cfg Config, log *zap.Logger) *RuntimeClient {
	return redisx.New(cfg, redisx.WithHooks(NewHook(log)))
}

func Init(ctx context.Context, cfg Config, log *zap.Logger) error {
	client := New(cfg, log)
	if err := client.Ping(ctx); err != nil {
		_ = client.Close()
		return err
	}

	replaceDefault(client)
	return nil
}

func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if Default == nil {
		RDB = nil
		return nil
	}
	err := Default.Close()
	Default = nil
	RDB = nil
	return err
}

func Raw() *goredis.Client {
	mu.Lock()
	defer mu.Unlock()
	return RDB
}

func Client(ctxs ...context.Context) *goredis.Client {
	_ = ctxs
	return Raw()
}

func ClientFrom(app *runtime.App) *goredis.Client {
	if client := From(app); client != nil {
		return client.Rdb
	}
	return Raw()
}

func Health(ctx context.Context) error {
	mu.Lock()
	client := Default
	mu.Unlock()
	if client == nil {
		return ErrNotInitialized
	}
	return client.Ping(ctx)
}
