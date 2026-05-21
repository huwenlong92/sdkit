package strategy

import (
	"sync"
	"time"

	"github.com/huwenlong92/sdkit/pkg/ratelimit/store"
)

// LeakyBucket 漏桶：请求先入桶，以固定速率漏出。超出容量则拒绝。
type LeakyBucket struct {
	rate     float64
	capacity int
	store    store.Store
	mu       sync.Mutex
	slots    map[string]*leakySlot
}

type leakySlot struct {
	last time.Time
	drop float64
}

// NewLeakyBucket 创建漏桶，store 为 nil 时默认使用内存存储
func NewLeakyBucket(rate float64, capacity int, stores ...store.Store) *LeakyBucket {
	s := pickStore(stores)
	lb := &LeakyBucket{rate: rate, capacity: capacity, store: s, slots: make(map[string]*leakySlot)}
	return lb
}

func (lb *LeakyBucket) Allow(key string) bool { return lb.AllowN(key, 1) }

func (lb *LeakyBucket) AllowN(key string, n int) bool {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	slot, ok := lb.slots[key]
	if !ok {
		slot = &leakySlot{last: time.Now()}
		lb.slots[key] = slot
	}

	now := time.Now()
	elapsed := now.Sub(slot.last).Seconds()
	drop := slot.drop - elapsed*lb.rate
	if drop < 0 {
		drop = 0
	}
	slot.last = now

	if drop+float64(n) > float64(lb.capacity) {
		slot.drop = drop
		return false
	}
	slot.drop = drop + float64(n)
	return true
}
