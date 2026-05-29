package risk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const defaultCounterPrefix = "security:risk:freq:"

var ErrCounterUnavailable = errors.New("risk: counter unavailable")

type Counter interface {
	Incr(ctx context.Context, key CounterKey) (int64, error)
}

type RedisCounterOption func(*RedisCounter)

type RedisCounter struct {
	client goredis.Cmdable
	prefix string
}

type CounterCache interface {
	Incr(ctx context.Context, key string) (int64, error)
	TTL(ctx context.Context, key string) (time.Duration, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
}

type CacheCounterOption func(*CacheCounter)

type CacheCounter struct {
	cache  CounterCache
	prefix string
}

var redisCounterScript = goredis.NewScript(`
redis.call("ZREMRANGEBYSCORE", KEYS[1], 0, ARGV[1] - ARGV[2])
local seq = redis.call("INCR", KEYS[2])
redis.call("ZADD", KEYS[1], ARGV[1], ARGV[1] .. "-" .. seq)
redis.call("EXPIRE", KEYS[1], ARGV[3])
redis.call("EXPIRE", KEYS[2], ARGV[3])
return redis.call("ZCARD", KEYS[1])
`)

func WithRedisCounterPrefix(prefix string) RedisCounterOption {
	return func(c *RedisCounter) {
		if prefix != "" {
			c.prefix = prefix
		}
	}
}

func NewRedisCounter(client goredis.Cmdable, opts ...RedisCounterOption) *RedisCounter {
	c := &RedisCounter{
		client: client,
		prefix: defaultCounterPrefix,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

func (c *RedisCounter) Incr(ctx context.Context, key CounterKey) (int64, error) {
	if c == nil || c.client == nil || key.WindowSeconds <= 0 {
		return 0, ErrCounterUnavailable
	}
	nowMs := time.Now().UnixMilli()
	windowMs := int64(key.WindowSeconds) * int64(time.Second/time.Millisecond)
	ttlSeconds := key.WindowSeconds + 1
	zsetKey, seqKey := counterRedisKeys(c.prefix, key)
	return redisCounterScript.Run(ctx, c.client, []string{zsetKey, seqKey}, nowMs, windowMs, ttlSeconds).Int64()
}

func WithCounterPrefix(prefix string) CacheCounterOption {
	return func(c *CacheCounter) {
		if prefix != "" {
			c.prefix = prefix
		}
	}
}

func NewCacheCounter(cache CounterCache, opts ...CacheCounterOption) *CacheCounter {
	c := &CacheCounter{
		cache:  cache,
		prefix: defaultCounterPrefix,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

func (c *CacheCounter) Incr(ctx context.Context, key CounterKey) (int64, error) {
	if c == nil || c.cache == nil || key.WindowSeconds <= 0 {
		return 0, ErrCounterUnavailable
	}
	cacheKey, _ := counterRedisKeys(c.prefix, key)
	count, err := c.cache.Incr(ctx, cacheKey)
	if err != nil {
		return 0, err
	}
	ttl, err := c.cache.TTL(ctx, cacheKey)
	if err == nil && ttl < 0 {
		_ = c.cache.Expire(ctx, cacheKey, time.Duration(key.WindowSeconds)*time.Second+time.Second)
	}
	return count, nil
}

func counterRedisKeys(prefix string, key CounterKey) (string, string) {
	event := strings.TrimSpace(key.Event)
	if event == "" {
		event = "*"
	}
	slot := hashParts(
		key.Service,
		key.Scene,
		event,
		key.TargetType,
		key.TargetValue,
		strconv.Itoa(key.WindowSeconds),
	)
	return prefix + "{" + slot + "}:z", prefix + "{" + slot + "}:seq"
}

func hashParts(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

var _ Counter = (*RedisCounter)(nil)
var _ Counter = (*CacheCounter)(nil)
