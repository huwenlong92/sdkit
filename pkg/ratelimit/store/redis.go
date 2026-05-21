package store

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore Redis 存储实现
type RedisStore struct {
	client *redis.Client
	prefix string
}

// NewRedisStore 创建 Redis 存储
func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client, prefix: "ratelimit:"}
}

// NewRedisStoreWithPrefix 创建带前缀的 Redis 存储
func NewRedisStoreWithPrefix(client *redis.Client, prefix string) *RedisStore {
	return &RedisStore{client: client, prefix: prefix}
}

// ======================== Counter ========================

// counterScript INCR + 首次 EXPIRE
var counterScript = redis.NewScript(`
	local count = redis.call('INCRBY', KEYS[1], ARGV[1])
	if count == tonumber(ARGV[1]) then
		redis.call('EXPIRE', KEYS[1], ARGV[2])
	end
	return count
`)

func (s *RedisStore) Counter(key string, n int, ttl time.Duration) (int, error) {
	return s.CounterContext(context.Background(), key, n, ttl)
}

func (s *RedisStore) CounterContext(ctx context.Context, key string, n int, ttl time.Duration) (int, error) {
	count, err := counterScript.Run(ctx, s.client, []string{s.prefix + "c:" + key}, n, ttlSeconds(ttl)).Int()
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ======================== WindowAdd ========================

// windowScript ZADD + ZREMRANGEBYSCORE + ZCARD
var windowScript = redis.NewScript(`
	local key = KEYS[1]
	local ts = tonumber(ARGV[1])
	local cutoff = tonumber(ARGV[2])
	local ttl = tonumber(ARGV[3])

	local n = tonumber(ARGV[4])

	for i = 1, n do
		-- 使用纳秒时间戳 + 自增序列作为 member，确保唯一
		local member = ts .. '-' .. redis.call('INCR', key .. ':seq')
		redis.call('ZADD', key, ts, member)
	end
	redis.call('ZREMRANGEBYSCORE', key, 0, cutoff)
	redis.call('EXPIRE', key, ttl)
	redis.call('EXPIRE', key .. ':seq', ttl)
	return redis.call('ZCARD', key)
`)

func (s *RedisStore) WindowAdd(key string, ts int64, n int, window time.Duration) (int, error) {
	return s.WindowAddContext(context.Background(), key, ts, n, window)
}

func (s *RedisStore) WindowAddContext(ctx context.Context, key string, ts int64, n int, window time.Duration) (int, error) {
	if n <= 0 {
		return 0, nil
	}
	ttl := ttlSeconds(window) + 1
	cutoff := ts - int64(window)
	count, err := windowScript.Run(ctx, s.client, []string{s.prefix + "w:" + key}, ts, cutoff, ttl, n).Int()
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ======================== TakeToken ========================

// tokenBucketScript Lua 令牌桶算法
var tokenBucketScript = redis.NewScript(`
	local key = KEYS[1]
	local rate = tonumber(ARGV[1])
	local burst = tonumber(ARGV[2])
	local now = tonumber(ARGV[3]) / 1e9  -- 纳秒转秒

	local data = redis.call('HMGET', key, 'tokens', 'last')
	local tokens = tonumber(data[1]) or burst
	local last = tonumber(data[2]) or now

	local elapsed = now - last
	tokens = math.min(burst, tokens + elapsed * rate)

	if tokens < 1 then
		redis.call('HMSET', key, 'tokens', tokens, 'last', now)
		redis.call('EXPIRE', key, math.ceil(burst / rate) + 1)
		return 0
	end

	tokens = tokens - 1
	redis.call('HMSET', key, 'tokens', tokens, 'last', now)
	redis.call('EXPIRE', key, math.ceil(burst / rate) + 1)
	return 1
`)

func (s *RedisStore) TakeToken(key string, rate float64, burst int) (bool, error) {
	return s.TakeTokenContext(context.Background(), key, rate, burst)
}

func (s *RedisStore) TakeTokenContext(ctx context.Context, key string, rate float64, burst int) (bool, error) {
	result, err := tokenBucketScript.Run(ctx, s.client, []string{s.prefix + "t:" + key}, rate, burst, time.Now().UnixNano()).Int()
	if err != nil {
		return false, fmt.Errorf("token bucket redis: %w", err)
	}
	return result == 1, nil
}

// ======================== Cleanup / Close ========================

func (s *RedisStore) Cleanup() {} // Redis key 自带 TTL，无需手动清理

func (s *RedisStore) Close() error {
	return nil
}

func ttlSeconds(ttl time.Duration) int {
	if ttl <= 0 {
		return 1
	}
	return int(math.Ceil(ttl.Seconds()))
}
