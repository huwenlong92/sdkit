package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"
	memorycontrol "github.com/huwenlong92/sdkit/pkg/queue/control/memory"
)

func TestMemoryLockerTokenGuard(t *testing.T) {
	locker := memorycontrol.NewLocker()
	unlock, ok, err := locker.Lock(context.Background(), "task:1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("Lock() ok=%v err=%v", ok, err)
	}
	if _, ok, err := locker.Lock(context.Background(), "task:1", time.Minute); !errors.Is(err, queue.ErrLockNotAcquired) || ok {
		t.Fatalf("second Lock() ok=%v err=%v", ok, err)
	}
	if err := unlock(context.Background()); err != nil {
		t.Fatalf("unlock error = %v", err)
	}
	if _, ok, err := locker.Lock(context.Background(), "task:1", time.Minute); err != nil || !ok {
		t.Fatalf("Lock after unlock ok=%v err=%v", ok, err)
	}
}

func TestMemoryIdempotency(t *testing.T) {
	store := memorycontrol.NewIdempotency()
	done, err := store.Done(context.Background(), "order:1")
	if err != nil || done {
		t.Fatalf("Done before MarkDone = %v, %v", done, err)
	}
	if err := store.MarkDone(context.Background(), "order:1", time.Minute); err != nil {
		t.Fatalf("MarkDone() error = %v", err)
	}
	done, err = store.Done(context.Background(), "order:1")
	if err != nil || !done {
		t.Fatalf("Done after MarkDone = %v, %v", done, err)
	}
}

func TestMemoryRateLimiter(t *testing.T) {
	limiter := memorycontrol.NewRateLimiter()
	ok, _, err := limiter.Allow(context.Background(), "tenant:1", 1, time.Minute)
	if err != nil || !ok {
		t.Fatalf("first Allow() ok=%v err=%v", ok, err)
	}
	ok, retryIn, err := limiter.Allow(context.Background(), "tenant:1", 1, time.Minute)
	if ok || !queue.IsRateLimitError(err) || retryIn <= 0 {
		t.Fatalf("second Allow() ok=%v retry=%s err=%v", ok, retryIn, err)
	}
}

func TestMemoryControlImplementsCoreInterfaces(t *testing.T) {
	var _ queue.Locker = memorycontrol.NewLocker()
	var _ queue.Idempotency = memorycontrol.NewIdempotency()
	var _ queue.RateLimiter = memorycontrol.NewRateLimiter()
}
