package tests

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/cache"
)

func testCache(t *testing.T, c cache.Cache) {
	ctx := context.Background()

	// Set + Get
	if err := c.Set(ctx, "foo", "bar", time.Minute); err != nil {
		t.Fatal(err)
	}
	v, err := c.Get(ctx, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if v != "bar" {
		t.Fatalf("Get: want bar, got %s", v)
	}

	// Get 不存在的 key
	v, err = c.Get(ctx, "no_key")
	if err != nil {
		t.Fatal(err)
	}
	if v != "" {
		t.Fatalf("Get no_key: want empty, got %s", v)
	}

	// Exists
	n, err := c.Exists(ctx, "foo", "no_key")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("Exists: want 1, got %d", n)
	}

	// Incr
	n2, err := c.Incr(ctx, "counter")
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 1 {
		t.Fatalf("Incr first: want 1, got %d", n2)
	}
	n2, err = c.Incr(ctx, "counter")
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 2 {
		t.Fatalf("Incr second: want 2, got %d", n2)
	}

	// TTL
	ttl, err := c.TTL(ctx, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if ttl <= 0 {
		t.Fatalf("TTL: want > 0, got %v", ttl)
	}
	ttl, err = c.TTL(ctx, "no_key")
	if err != nil {
		t.Fatal(err)
	}
	if ttl != -2 {
		t.Fatalf("TTL no_key: want -2, got %v", ttl)
	}

	// Del
	if err := c.Del(ctx, "foo", "counter"); err != nil {
		t.Fatal(err)
	}
	v, _ = c.Get(ctx, "foo")
	if v != "" {
		t.Fatal("Del: key should be deleted")
	}

	// Sets + Gets
	if err := c.Sets(ctx, map[string]string{"a": "1", "b": "2", "c": "3"}, time.Minute); err != nil {
		t.Fatal("Sets:", err)
	}
	got, missing := c.Gets(ctx, []string{"a", "b", "no1", "c", "no2"})
	if len(got) != 3 {
		t.Fatalf("Gets: want 3, got %d", len(got))
	}
	if got["a"] != "1" || got["b"] != "2" || got["c"] != "3" {
		t.Fatalf("Gets: wrong values: %v", got)
	}
	if len(missing) != 2 {
		t.Fatalf("Gets missing: want 2, got %d", len(missing))
	}

	// Gets empty
	got, missing = c.Gets(ctx, []string{})
	if len(got) != 0 || len(missing) != 0 {
		t.Fatal("Gets empty should return empty")
	}

	// Sets empty
	if err := c.Sets(ctx, map[string]string{}, time.Minute); err != nil {
		t.Fatal("Sets empty should not error:", err)
	}

	// Delete
	c.Sets(ctx, map[string]string{"d1": "x", "d2": "y"}, time.Minute)
	if err := c.Delete(ctx, []string{"d1", "d2", "no_key"}); err != nil {
		t.Fatal("Delete:", err)
	}
	v, _ = c.Get(ctx, "d1")
	if v != "" {
		t.Fatal("Delete: d1 should be gone")
	}
	v, _ = c.Get(ctx, "d2")
	if v != "" {
		t.Fatal("Delete: d2 should be gone")
	}

	// Delete empty
	if err := c.Delete(ctx, []string{}); err != nil {
		t.Fatal("Delete empty should not error:", err)
	}

	// Expiry
	if err := c.Set(ctx, "expire", "x", 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	v, _ = c.Get(ctx, "expire")
	if v != "" {
		t.Fatal("expired key should be gone")
	}
}

func TestMemoryCache(t *testing.T) {
	c := cache.New()
	defer c.Close()
	testCache(t, c)
}

func TestJSONHelpers(t *testing.T) {
	c := cache.New()
	defer c.Close()
	ctx := context.Background()

	type profile struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}

	want := profile{ID: 1, Name: "admin"}
	if err := cache.Set(ctx, c, cache.UserKey(want.ID), want, time.Minute); err != nil {
		t.Fatal(err)
	}

	var got profile
	if err := cache.Get(ctx, c, cache.UserKey(want.ID), &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Get JSON: want %+v, got %+v", want, got)
	}

	var missing profile
	if err := cache.Get(ctx, c, cache.UserKey(2), &missing); !errors.Is(err, cache.ErrNotFound) {
		t.Fatalf("Get missing: want ErrNotFound, got %v", err)
	}

	ok, err := cache.GetJSON(ctx, c, cache.UserKey(2), &missing)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("GetJSON missing: want ok=false")
	}

	if key := cache.Key("user", 1, ":profile:"); key != "user:1:profile" {
		t.Fatalf("Key: got %q", key)
	}
}

func TestRemember(t *testing.T) {
	c := cache.New()
	defer c.Close()
	ctx := context.Background()

	type profile struct {
		ID int64 `json:"id"`
	}

	var calls int64
	loaded, err := cache.Remember(ctx, c, cache.UserKey(1), time.Minute, func() (profile, error) {
		atomic.AddInt64(&calls, 1)
		return profile{ID: 1}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != 1 {
		t.Fatalf("Remember first: got %+v", loaded)
	}

	loaded, err = cache.Remember(ctx, c, cache.UserKey(1), time.Minute, func() (profile, error) {
		atomic.AddInt64(&calls, 1)
		return profile{ID: 2}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != 1 {
		t.Fatalf("Remember cached: got %+v", loaded)
	}
	if calls != 1 {
		t.Fatalf("Remember calls: want 1, got %d", calls)
	}
}

func TestRememberSingleflight(t *testing.T) {
	c := cache.New()
	defer c.Close()
	ctx := context.Background()
	key := cache.Key("singleflight", time.Now().UnixNano())

	var calls int64
	var wg sync.WaitGroup
	errCh := make(chan error, 16)
	start := make(chan struct{})

	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			v, err := cache.Remember(ctx, c, key, time.Minute, func() (int, error) {
				atomic.AddInt64(&calls, 1)
				time.Sleep(10 * time.Millisecond)
				return 7, nil
			})
			if err != nil {
				errCh <- err
				return
			}
			if v != 7 {
				errCh <- errors.New("unexpected remember value")
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("singleflight calls: want 1, got %d", calls)
	}
}
