package risk2

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"time"
)

const defaultStoreCacheTTL = 10 * time.Second

type StoreCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}

type StoreCacheOption func(*cachedStore)

type cachedStore struct {
	next  Store
	cache StoreCache
	ttl   time.Duration
}

type cachedValue[T any] struct {
	Hit   bool `json:"hit"`
	Value T    `json:"value,omitempty"`
}

func WithStoreCacheTTL(ttl time.Duration) StoreCacheOption {
	return func(s *cachedStore) {
		if ttl > 0 {
			s.ttl = ttl
		}
	}
}

func NewCachedStore(next Store, cache StoreCache, opts ...StoreCacheOption) Store {
	if next == nil || cache == nil {
		return next
	}
	s := &cachedStore{
		next:  next,
		cache: cache,
		ttl:   defaultStoreCacheTTL,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

func (s *cachedStore) LoadScene(ctx context.Context, service string, scene string) (*Scene, error) {
	key := riskCacheKey("scene", service, scene)
	if cached, ok := cacheRead[*Scene](ctx, s.cache, key); ok {
		if !cached.Hit {
			return nil, nil
		}
		return cached.Value, nil
	}

	value, err := s.next.LoadScene(ctx, service, scene)
	if err != nil {
		return nil, err
	}
	cacheWrite(ctx, s.cache, key, cachedValue[*Scene]{Hit: value != nil, Value: value}, s.ttl)
	return value, nil
}

func (s *cachedStore) MatchList(ctx context.Context, event Event, listType string) (*ListRule, error) {
	key := riskCacheKey("list", event.Service, event.Scene, listType, targetSignature(event))
	if cached, ok := cacheRead[*ListRule](ctx, s.cache, key); ok {
		if !cached.Hit {
			return nil, nil
		}
		return cached.Value, nil
	}

	value, err := s.next.MatchList(ctx, event, listType)
	if err != nil {
		return nil, err
	}
	cacheWrite(ctx, s.cache, key, cachedValue[*ListRule]{Hit: value != nil, Value: value}, s.ttl)
	return value, nil
}

func (s *cachedStore) ListFrequencyRules(ctx context.Context, event Event) ([]FrequencyRule, error) {
	key := riskCacheKey("frequency", event.Service, event.Scene, event.Event)
	if cached, ok := cacheRead[[]FrequencyRule](ctx, s.cache, key); ok {
		if cached.Value == nil {
			return []FrequencyRule{}, nil
		}
		return cached.Value, nil
	}

	value, err := s.next.ListFrequencyRules(ctx, event)
	if err != nil {
		return nil, err
	}
	cacheWrite(ctx, s.cache, key, cachedValue[[]FrequencyRule]{Hit: true, Value: value}, s.ttl)
	return value, nil
}

func (s *cachedStore) CountEvents(ctx context.Context, query EventCountQuery) (int64, error) {
	return s.next.CountEvents(ctx, query)
}

func (s *cachedStore) SaveDecision(ctx context.Context, event Event, decision *Decision) error {
	return s.next.SaveDecision(ctx, event, decision)
}

func cacheRead[T any](ctx context.Context, cache StoreCache, key string) (cachedValue[T], bool) {
	var value cachedValue[T]
	raw, err := cache.Get(ctx, key)
	if err != nil || raw == "" {
		return value, false
	}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return value, false
	}
	return value, true
}

func cacheWrite[T any](ctx context.Context, cache StoreCache, key string, value cachedValue[T], ttl time.Duration) {
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	_ = cache.Set(ctx, key, string(raw), ttl)
}

func riskCacheKey(kind string, parts ...string) string {
	return "security:risk2:config:" + kind + ":" + hashParts(parts...)
}

func targetSignature(event Event) string {
	targets := TargetValues(event)
	if len(targets) == 0 {
		return ""
	}
	keys := make([]string, 0, len(targets))
	for key := range targets {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		parts = append(parts, key, targets[key])
	}
	return strconv.Itoa(len(keys)) + ":" + hashParts(parts...)
}

var _ Store = (*cachedStore)(nil)
