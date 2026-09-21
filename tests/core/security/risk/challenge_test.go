package risk_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/security/risk"
	"github.com/redis/go-redis/v9"
)

func TestChallengeLifecycle(t *testing.T) {
	socket := os.Getenv("SDKIT_RISK_TEST_REDIS_SOCKET")
	if socket == "" {
		t.Skip("set SDKIT_RISK_TEST_REDIS_SOCKET to a disposable Redis Unix socket")
	}
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	store := risk.NewChallengeStore(client)
	account := risk.ChallengeKey{Service: "test", Scene: "login", Rule: fmt.Sprintf("fixture_%d", time.Now().UnixNano()), TargetType: "account", TargetValue: "fictional-user"}
	ip := account
	ip.TargetType = "ip"
	ip.TargetValue = "192.0.2.12"
	require := func(key risk.ChallengeKey) {
		t.Helper()
		if err := store.Require(ctx, key, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	read := func() []risk.ChallengeRequirement {
		t.Helper()
		rows, err := store.Get(ctx, []risk.ChallengeKey{account, ip})
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	if got := len(read()); got != 0 {
		t.Fatalf("initial requirements=%d, want 0", got)
	}
	require(account)
	require(ip)
	initial := read()
	if len(initial) != 2 {
		t.Fatalf("requirements=%d, want 2", len(initial))
	}
	again := read()
	if again[0].Version != initial[0].Version {
		t.Fatal("read changed requirement version")
	}
	require(account)
	if err := store.Resolve(ctx, initial[0]); err != nil {
		t.Fatal(err)
	}
	current := read()
	if len(current) != 2 || current[0].Version == initial[0].Version {
		t.Fatal("concurrent requirement was lost")
	}
	if err := store.Resolve(ctx, current[0]); err != nil {
		t.Fatal(err)
	}
	remaining := read()
	if len(remaining) != 1 || remaining[0].Key.TargetType != "ip" {
		t.Fatal("account resolution cleared IP state")
	}
	if err := store.Resolve(ctx, remaining[0]); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan bool, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := store.ClaimProof(ctx, account.Rule, "fixture-proof", time.Minute)
			if err != nil {
				t.Error(err)
			}
			results <- ok
		}()
	}
	wg.Wait()
	close(results)
	claimed := 0
	for ok := range results {
		if ok {
			claimed++
		}
	}
	if claimed != 1 {
		t.Fatalf("proof claims=%d, want 1", claimed)
	}
	if err := store.Require(ctx, account, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(40 * time.Millisecond)
	defer timer.Stop()
	<-timer.C
	if got := len(read()); got != 0 {
		t.Fatalf("expired requirements=%d, want 0", got)
	}
	if err := store.Require(ctx, account, 0); err == nil {
		t.Fatal("zero TTL accepted")
	}
}
