// Package store 提供限流状态存储后端（内存 / Redis）
package store

import (
	"context"
	"time"
)

// Store 限流状态存储接口，每种策略使用对应方法
type Store interface {
	// Counter 固定窗口：原子递增 key 计数，返回计数值。首次调用设置 TTL。
	Counter(key string, n int, ttl time.Duration) (int, error)

	// WindowAdd 滑动窗口：追加时间戳，返回窗口内计数。
	WindowAdd(key string, ts int64, n int, window time.Duration) (int, error)

	// TakeToken 令牌桶：消费一个令牌，返回是否允许。
	TakeToken(key string, rate float64, burst int) (bool, error)

	// Cleanup 清理过期数据
	Cleanup()

	// Close 关闭连接
	Close() error
}

type ContextStore interface {
	Store
	CounterContext(ctx context.Context, key string, n int, ttl time.Duration) (int, error)
	WindowAddContext(ctx context.Context, key string, ts int64, n int, window time.Duration) (int, error)
	TakeTokenContext(ctx context.Context, key string, rate float64, burst int) (bool, error)
}
