package redisx

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
)

type Client struct {
	Rdb    *redis.Client
	Prefix string
}

type Option func(*options)

type options struct {
	hooks []redis.Hook
}

func WithHooks(hooks ...redis.Hook) Option {
	return func(o *options) {
		o.hooks = append(o.hooks, hooks...)
	}
}

func New(cfg Config, opts ...Option) *Client {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		MaintNotificationsConfig: &maintnotifications.Config{
			Mode: maintnotifications.ModeDisabled,
		},
	})
	for _, hook := range o.hooks {
		if hook != nil {
			rdb.AddHook(hook)
		}
	}

	return &Client{
		Rdb:    rdb,
		Prefix: cfg.Prefix,
	}
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.Rdb == nil {
		return nil
	}
	return c.Rdb.Ping(ctx).Err()
}

func (c *Client) Close() error {
	if c == nil || c.Rdb == nil {
		return nil
	}
	return c.Rdb.Close()
}
