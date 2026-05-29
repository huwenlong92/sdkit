package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"github.com/huwenlong92/sdkit/core/security/risk"
)

const defaultTTL = 10 * time.Second

type Backend interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}

type Option func(*store)

type store struct {
	next  risk.Store
	cache Backend
	ttl   time.Duration
}

type cachedValue[T any] struct {
	Hit   bool `json:"hit"`
	Value T    `json:"value,omitempty"`
}

func WithTTL(ttl time.Duration) Option {
	return func(s *store) {
		if ttl > 0 {
			s.ttl = ttl
		}
	}
}

func New(next risk.Store, cache Backend, opts ...Option) risk.Store {
	if next == nil || cache == nil {
		return next
	}
	s := &store{
		next:  next,
		cache: cache,
		ttl:   defaultTTL,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

func (s *store) LoadScene(ctx context.Context, service string, scene string) (*risk.Scene, error) {
	key := riskCacheKey("scene", service, scene)
	if cached, ok := cacheRead[*risk.Scene](ctx, s.cache, key); ok {
		if !cached.Hit {
			return nil, nil
		}
		return cached.Value, nil
	}

	value, err := s.next.LoadScene(ctx, service, scene)
	if err != nil {
		return nil, err
	}
	cacheWrite(ctx, s.cache, key, cachedValue[*risk.Scene]{Hit: value != nil, Value: value}, s.ttl)
	return value, nil
}

func (s *store) MatchList(ctx context.Context, event risk.Event, listType string) (*risk.ListRule, error) {
	key := riskCacheKey("list", event.Service, event.Scene, listType, targetSignature(event))
	if cached, ok := cacheRead[*risk.ListRule](ctx, s.cache, key); ok {
		if !cached.Hit {
			return nil, nil
		}
		return cached.Value, nil
	}

	value, err := s.next.MatchList(ctx, event, listType)
	if err != nil {
		return nil, err
	}
	cacheWrite(ctx, s.cache, key, cachedValue[*risk.ListRule]{Hit: value != nil, Value: value}, s.ttl)
	return value, nil
}

func (s *store) ListFrequencyRules(ctx context.Context, event risk.Event) ([]risk.FrequencyRule, error) {
	key := riskCacheKey("frequency", event.Service, event.Scene, event.Event)
	if cached, ok := cacheRead[[]risk.FrequencyRule](ctx, s.cache, key); ok {
		if cached.Value == nil {
			return []risk.FrequencyRule{}, nil
		}
		return cached.Value, nil
	}

	value, err := s.next.ListFrequencyRules(ctx, event)
	if err != nil {
		return nil, err
	}
	cacheWrite(ctx, s.cache, key, cachedValue[[]risk.FrequencyRule]{Hit: true, Value: value}, s.ttl)
	return value, nil
}

func (s *store) CountEvents(ctx context.Context, query risk.EventCountQuery) (int64, error) {
	return s.next.CountEvents(ctx, query)
}

func (s *store) SaveDecision(ctx context.Context, event risk.Event, decision *risk.Decision) error {
	return s.next.SaveDecision(ctx, event, decision)
}

func cacheRead[T any](ctx context.Context, cache Backend, key string) (cachedValue[T], bool) {
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

func cacheWrite[T any](ctx context.Context, cache Backend, key string, value cachedValue[T], ttl time.Duration) {
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	_ = cache.Set(ctx, key, string(raw), ttl)
}

func riskCacheKey(kind string, parts ...string) string {
	return "security:risk:config:" + kind + ":" + hashParts(parts...)
}

func targetSignature(event risk.Event) string {
	targets := risk.TargetValues(event)
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

func hashParts(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

var _ risk.Store = (*store)(nil)
