package memory

import (
	"context"
	"sync"
	"time"

	pkgcache "github.com/huwenlong92/sdkit/pkg/cache"
)

var _ pkgcache.Cache = (*Store)(nil)

type memItem struct {
	value  string
	expire time.Time
}

type Store struct {
	mu     sync.RWMutex
	items  map[string]*memItem
	stopCh chan struct{}
}

func New() *Store {
	m := &Store{
		items:  make(map[string]*memItem),
		stopCh: make(chan struct{}),
	}
	go m.cleanup()
	return m
}

func (m *Store) Get(_ context.Context, key string) (string, error) {
	m.mu.RLock()
	item, ok := m.items[key]
	m.mu.RUnlock()

	if !ok {
		return "", nil
	}
	if !item.expire.IsZero() && time.Now().After(item.expire) {
		m.mu.Lock()
		delete(m.items, key)
		m.mu.Unlock()
		return "", nil
	}
	return item.value, nil
}

func (m *Store) Set(_ context.Context, key, value string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var expire time.Time
	if ttl > 0 {
		expire = time.Now().Add(ttl)
	}
	m.items[key] = &memItem{value: value, expire: expire}
	return nil
}

func (m *Store) Del(_ context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		delete(m.items, key)
	}
	return nil
}

func (m *Store) Exists(_ context.Context, keys ...string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int64
	now := time.Now()
	for _, key := range keys {
		if item, ok := m.items[key]; ok {
			if item.expire.IsZero() || now.Before(item.expire) {
				count++
			}
		}
	}
	return count, nil
}

func (m *Store) Incr(_ context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.items[key]
	if !ok || (!item.expire.IsZero() && time.Now().After(item.expire)) {
		m.items[key] = &memItem{value: "1"}
		return 1, nil
	}

	var n int64
	for _, c := range item.value {
		if c >= '0' && c <= '9' {
			n = n*10 + int64(c-'0')
		}
	}
	n++
	item.value = formatInt(n)
	return n, nil
}

func (m *Store) TTL(_ context.Context, key string) (time.Duration, error) {
	m.mu.RLock()
	item, ok := m.items[key]
	m.mu.RUnlock()

	if !ok {
		return -2, nil
	}
	if item.expire.IsZero() {
		return -1, nil
	}
	d := time.Until(item.expire)
	if d <= 0 {
		return -2, nil
	}
	return d, nil
}

func (m *Store) Gets(_ context.Context, keys []string) (map[string]string, []string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]string, len(keys))
	var missing []string
	now := time.Now()
	for _, key := range keys {
		item, ok := m.items[key]
		if !ok {
			missing = append(missing, key)
			continue
		}
		if !item.expire.IsZero() && now.After(item.expire) {
			missing = append(missing, key)
			continue
		}
		result[key] = item.value
	}
	return result, missing
}

func (m *Store) Sets(_ context.Context, values map[string]string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var expire time.Time
	if ttl > 0 {
		expire = time.Now().Add(ttl)
	}
	for k, v := range values {
		m.items[k] = &memItem{value: v, expire: expire}
	}
	return nil
}

func (m *Store) Delete(_ context.Context, keys []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		delete(m.items, key)
	}
	return nil
}

func (m *Store) Close() error {
	select {
	case <-m.stopCh:
	default:
		close(m.stopCh)
	}
	return nil
}

func (m *Store) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.mu.Lock()
			now := time.Now()
			for k, item := range m.items {
				if !item.expire.IsZero() && now.After(item.expire) {
					delete(m.items, k)
				}
			}
			m.mu.Unlock()
		case <-m.stopCh:
			return
		}
	}
}

func formatInt(n int64) string {
	if n == 0 {
		return "0"
	}
	s := make([]byte, 0, 20)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		s = append([]byte{byte('0' + n%10)}, s...)
		n /= 10
	}
	if neg {
		s = append([]byte{'-'}, s...)
	}
	return string(s)
}
