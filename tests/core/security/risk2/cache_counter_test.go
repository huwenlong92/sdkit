package risk2_test

import (
	"context"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/cache"
	corerisk "github.com/huwenlong92/sdkit/core/security/risk2"
)

func TestCacheCounterIncrementsWithTTL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := cache.New()
	defer c.Close()
	counter := corerisk.NewCacheCounter(c)

	key := corerisk.CounterKey{
		Service:       "admin",
		Scene:         "login",
		Event:         "login_failed",
		TargetType:    "account",
		TargetValue:   "admin",
		WindowSeconds: 60,
	}

	count, err := counter.Incr(ctx, key)
	if err != nil {
		t.Fatalf("first incr failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("first count = %d, want 1", count)
	}

	count, err = counter.Incr(ctx, key)
	if err != nil {
		t.Fatalf("second incr failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("second count = %d, want 2", count)
	}
}

func TestRedisCounterRequiresClient(t *testing.T) {
	t.Parallel()

	counter := corerisk.NewRedisCounter(nil)
	_, err := counter.Incr(context.Background(), corerisk.CounterKey{
		Service:       "admin",
		Scene:         "login",
		Event:         "login_failed",
		TargetType:    "account",
		TargetValue:   "admin",
		WindowSeconds: 60,
	})
	if err != corerisk.ErrCounterUnavailable {
		t.Fatalf("err = %v, want %v", err, corerisk.ErrCounterUnavailable)
	}
}

func TestEngineUsesCounterForFrequencyRules(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := cache.New()
	defer c.Close()
	store := &fakeStore{
		rules: []corerisk.FrequencyRule{
			{
				ID:            10,
				Code:          "login_failed_account",
				Name:          "登录失败账号频率",
				Event:         "login_failed",
				TargetType:    "account",
				WindowSeconds: 60,
				LimitCount:    2,
				Action:        corerisk.ActionLimit,
				Score:         50,
			},
		},
	}
	engine := corerisk.NewEngine(store, corerisk.WithCounter(corerisk.NewCacheCounter(c)))

	event := corerisk.Event{
		Service: "admin",
		Scene:   "login",
		Event:   "login_failed",
		Extra: map[string]any{
			"account": "admin",
		},
	}

	first, err := engine.Evaluate(ctx, event)
	if err != nil {
		t.Fatalf("first evaluate failed: %v", err)
	}
	if !first.Passed || len(first.Hits) != 0 {
		t.Fatalf("first decision passed=%v hits=%d, want passed without hits", first.Passed, len(first.Hits))
	}

	second, err := engine.Evaluate(ctx, event)
	if err != nil {
		t.Fatalf("second evaluate failed: %v", err)
	}
	if second.Passed || len(second.Hits) != 1 {
		t.Fatalf("second decision passed=%v hits=%d, want blocked with one hit", second.Passed, len(second.Hits))
	}
	if store.countCalls != 0 {
		t.Fatalf("db count calls = %d, want 0", store.countCalls)
	}
}

func TestCachedStoreCachesRuntimeConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := cache.New()
	defer c.Close()
	store := &fakeStore{
		scene: &corerisk.Scene{
			Service:       "admin",
			Code:          "login",
			DefaultAction: corerisk.ActionAllow,
		},
		listRule: &corerisk.ListRule{
			ID:          1,
			ListType:    "black",
			TargetType:  "account",
			TargetValue: "admin",
		},
		rules: []corerisk.FrequencyRule{{ID: 2, Code: "rule"}},
	}
	cached := corerisk.NewCachedStore(store, c, corerisk.WithStoreCacheTTL(time.Minute))
	event := corerisk.Event{
		Service: "admin",
		Scene:   "login",
		Event:   "login_failed",
		Extra: map[string]any{
			"account": "admin",
		},
	}

	if _, err := cached.LoadScene(ctx, "admin", "login"); err != nil {
		t.Fatalf("load scene failed: %v", err)
	}
	if _, err := cached.LoadScene(ctx, "admin", "login"); err != nil {
		t.Fatalf("load scene from cache failed: %v", err)
	}
	if _, err := cached.MatchList(ctx, event, "black"); err != nil {
		t.Fatalf("match list failed: %v", err)
	}
	if _, err := cached.MatchList(ctx, event, "black"); err != nil {
		t.Fatalf("match list from cache failed: %v", err)
	}
	if _, err := cached.ListFrequencyRules(ctx, event); err != nil {
		t.Fatalf("list frequency failed: %v", err)
	}
	if _, err := cached.ListFrequencyRules(ctx, event); err != nil {
		t.Fatalf("list frequency from cache failed: %v", err)
	}

	if store.sceneCalls != 1 {
		t.Fatalf("scene calls = %d, want 1", store.sceneCalls)
	}
	if store.listCalls != 1 {
		t.Fatalf("list calls = %d, want 1", store.listCalls)
	}
	if store.ruleCalls != 1 {
		t.Fatalf("rule calls = %d, want 1", store.ruleCalls)
	}
}

type fakeStore struct {
	scene    *corerisk.Scene
	listRule *corerisk.ListRule
	rules    []corerisk.FrequencyRule

	sceneCalls int
	listCalls  int
	ruleCalls  int
	countCalls int
}

func (s *fakeStore) LoadScene(_ context.Context, _ string, _ string) (*corerisk.Scene, error) {
	s.sceneCalls++
	return s.scene, nil
}

func (s *fakeStore) MatchList(_ context.Context, _ corerisk.Event, _ string) (*corerisk.ListRule, error) {
	s.listCalls++
	return s.listRule, nil
}

func (s *fakeStore) ListFrequencyRules(_ context.Context, _ corerisk.Event) ([]corerisk.FrequencyRule, error) {
	s.ruleCalls++
	return s.rules, nil
}

func (s *fakeStore) CountEvents(_ context.Context, _ corerisk.EventCountQuery) (int64, error) {
	s.countCalls++
	return 0, nil
}

func (s *fakeStore) SaveDecision(_ context.Context, _ corerisk.Event, _ *corerisk.Decision) error {
	return nil
}
