package tests

import (
	"context"
	"errors"
	"fmt"
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

	// Expire
	if err := c.Expire(ctx, "foo", 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	v, err = c.Get(ctx, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if v != "" {
		t.Fatal("Expire: key should be expired")
	}
	if err := c.Set(ctx, "persist", "x", 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := c.Expire(ctx, "persist", 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	v, err = c.Get(ctx, "persist")
	if err != nil {
		t.Fatal(err)
	}
	if v != "x" {
		t.Fatal("Expire ttl<=0 should clear expiration")
	}

	// Del
	if err := c.Del(ctx, "persist", "counter"); err != nil {
		t.Fatal(err)
	}
	v, _ = c.Get(ctx, "persist")
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

func TestDefaultOperations(t *testing.T) {
	c := cache.New()
	t.Cleanup(cache.Close)
	if err := cache.Bind(nil, c); err != nil {
		t.Fatal(err)
	}
	testDefaultOperations(t)
}

func TestWithOperations(t *testing.T) {
	c := cache.New()
	defer c.Close()
	testWithOperations(t, c)
}

func testDefaultOperations(t *testing.T) {
	ctx := context.Background()

	if err := cache.Set(ctx, "pkg:foo", "bar", time.Minute); err != nil {
		t.Fatal(err)
	}
	got, err := cache.Get(ctx, "pkg:foo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "bar" {
		t.Fatalf("Get: want bar, got %q", got)
	}

	n, err := cache.Exists(ctx, "pkg:foo", "pkg:missing")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("Exists: want 1, got %d", n)
	}

	n, err = cache.Incr(ctx, "pkg:counter")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("Incr: want 1, got %d", n)
	}

	ttl, err := cache.TTL(ctx, "pkg:foo")
	if err != nil {
		t.Fatal(err)
	}
	if ttl <= 0 {
		t.Fatalf("TTL: want > 0, got %v", ttl)
	}

	if err := cache.Expire(ctx, "pkg:foo", 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	got, err = cache.Get(ctx, "pkg:foo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatal("Expire: key should be expired")
	}

	if err := cache.Sets(ctx, map[string]string{"pkg:a": "1", "pkg:b": "2"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	values, missing := cache.Gets(ctx, []string{"pkg:a", "pkg:b", "pkg:c"})
	if values["pkg:a"] != "1" || values["pkg:b"] != "2" || len(missing) != 1 {
		t.Fatalf("Gets: values=%v missing=%v", values, missing)
	}

	if err := cache.Del(ctx, "pkg:counter"); err != nil {
		t.Fatal(err)
	}
	if err := cache.Delete(ctx, []string{"pkg:a", "pkg:b"}); err != nil {
		t.Fatal(err)
	}
}

func testWithOperations(t *testing.T, c cache.Cache) {
	ctx := context.Background()

	if err := cache.SetWith(ctx, c, "with:foo", "bar", time.Minute); err != nil {
		t.Fatal(err)
	}
	got, err := cache.GetWith(ctx, c, "with:foo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "bar" {
		t.Fatalf("GetWith: want bar, got %q", got)
	}

	n, err := cache.ExistsWith(ctx, c, "with:foo", "with:missing")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("ExistsWith: want 1, got %d", n)
	}

	n, err = cache.IncrWith(ctx, c, "with:counter")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("IncrWith: want 1, got %d", n)
	}

	ttl, err := cache.TTLWith(ctx, c, "with:foo")
	if err != nil {
		t.Fatal(err)
	}
	if ttl <= 0 {
		t.Fatalf("TTLWith: want > 0, got %v", ttl)
	}

	if err := cache.ExpireWith(ctx, c, "with:foo", 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	got, err = cache.GetWith(ctx, c, "with:foo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatal("ExpireWith: key should be expired")
	}

	if err := cache.SetsWith(ctx, c, map[string]string{"with:a": "1", "with:b": "2"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	values, missing := cache.GetsWith(ctx, c, []string{"with:a", "with:b", "with:c"})
	if values["with:a"] != "1" || values["with:b"] != "2" || len(missing) != 1 {
		t.Fatalf("GetsWith: values=%v missing=%v", values, missing)
	}

	if err := cache.DelWith(ctx, c, "with:counter"); err != nil {
		t.Fatal(err)
	}
	if err := cache.DeleteWith(ctx, c, []string{"with:a", "with:b"}); err != nil {
		t.Fatal(err)
	}
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
	key := fmt.Sprintf("user:%d", want.ID)
	if err := cache.SetJSONWith(ctx, c, key, want, time.Minute); err != nil {
		t.Fatal(err)
	}

	var got profile
	ok, err := cache.GetJSONWith(ctx, c, key, &got)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("GetJSONWith: want ok=true")
	}
	if got != want {
		t.Fatalf("Get JSON: want %+v, got %+v", want, got)
	}

	var missing profile
	ok, err = cache.GetJSONWith(ctx, c, "user:2", &missing)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("GetJSON missing: want ok=false")
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
	key := "user:1"
	loaded, err := cache.RememberWith(ctx, c, key, time.Minute, func() (profile, error) {
		atomic.AddInt64(&calls, 1)
		return profile{ID: 1}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != 1 {
		t.Fatalf("Remember first: got %+v", loaded)
	}

	loaded, err = cache.RememberWith(ctx, c, key, time.Minute, func() (profile, error) {
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
	key := fmt.Sprintf("singleflight:%d", time.Now().UnixNano())

	var calls int64
	var wg sync.WaitGroup
	errCh := make(chan error, 16)
	start := make(chan struct{})

	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			v, err := cache.RememberWith(ctx, c, key, time.Minute, func() (int, error) {
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
