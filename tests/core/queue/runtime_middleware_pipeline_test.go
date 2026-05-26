package queue_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/queue/runtime/middleware"

	"go.uber.org/zap"
)

func TestDispatcherRuntimePipelineUsesMiddlewareMetadata(t *testing.T) {
	dispatcher := queue.NewDispatcher()
	order := make([]string, 0, 4)

	dispatcher.Use(
		func(next queue.HandlerFunc) queue.HandlerFunc {
			return func(ctx context.Context, msg *queue.Message) error {
				order = append(order, "global:before")
				err := next(ctx, msg)
				order = append(order, "global:after")
				return err
			}
		},
	)

	err := dispatcher.Register(
		"user.sync",
		func(next queue.HandlerFunc) queue.HandlerFunc {
			return func(ctx context.Context, msg *queue.Message) error {
				order = append(order, "local:before")
				err := next(ctx, msg)
				order = append(order, "local:after")
				return err
			}
		},
		func(ctx context.Context, msg *queue.Message) error {
			timeout, ok := queue.MessageMetadataDuration(msg, queue.MessageMetadataTimeout)
			if !ok || timeout != 10*time.Millisecond {
				t.Fatalf("message timeout metadata = %s, ok=%v", timeout, ok)
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("timeout middleware did not attach deadline")
			}
			<-ctx.Done()
			return ctx.Err()
		},
		queue.WithTimeout(10*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	err = dispatcher.Dispatch(context.Background(), "user.sync", &queue.Message{ID: "task-1"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("dispatch error = %v, want deadline exceeded", err)
	}
	wantOrder := []string{"global:before", "local:before", "local:after", "global:after"}
	if strings.Join(order, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("order = %v, want %v", order, wantOrder)
	}
}

func TestDispatcherDoesNotAttachTimeoutWithoutTaskTimeout(t *testing.T) {
	dispatcher := queue.NewDispatcher()
	if err := dispatcher.Register("user.no_timeout", func(ctx context.Context, msg *queue.Message) error {
		if _, ok := ctx.Deadline(); ok {
			t.Fatal("task without queue.WithTimeout should not get deadline")
		}
		return nil
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := dispatcher.Dispatch(context.Background(), "user.no_timeout", &queue.Message{ID: "task-1"}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
}

func TestRuntimeMiddlewareRecoverConvertsPanicToError(t *testing.T) {
	handler := middleware.Recover(zap.NewNop())(func(context.Context, *queue.Message) error {
		panic("boom")
	})

	err := handler(context.Background(), &queue.Message{Type: "panic.task"})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("recover error = %v", err)
	}
}

func TestRuntimeMiddlewareLockUsesQueueLocker(t *testing.T) {
	locker := &runtimeLocker{}
	called := false
	handler := middleware.Lock(locker, middleware.StaticLockKey("task:1", time.Second))(
		func(context.Context, *queue.Message) error {
			called = true
			return nil
		},
	)

	if err := handler(context.Background(), &queue.Message{Type: "lock.task"}); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !called || !locker.locked || !locker.unlocked {
		t.Fatalf("lock pipeline called=%v locked=%v unlocked=%v", called, locker.locked, locker.unlocked)
	}
}

func TestRuntimeMiddlewareLockCanReadMessageMetadata(t *testing.T) {
	locker := &runtimeLocker{}
	handler := middleware.Lock(locker)(func(context.Context, *queue.Message) error {
		return nil
	})

	msg := &queue.Message{Type: "lock.task"}
	queue.SetMessageMetadata(msg, queue.MessageMetadataLockKey, "task:1")
	queue.SetMessageMetadata(msg, queue.MessageMetadataLockTTL, time.Second)

	if err := handler(context.Background(), msg); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !locker.locked || !locker.unlocked {
		t.Fatalf("metadata lock locked=%v unlocked=%v", locker.locked, locker.unlocked)
	}
}

func TestRuntimeMiddlewareLockReturnsNotAcquired(t *testing.T) {
	locker := &runtimeLocker{conflict: true}
	called := false
	handler := middleware.Lock(locker, middleware.StaticLockKey("task:1", time.Second))(
		func(context.Context, *queue.Message) error {
			called = true
			return nil
		},
	)

	err := handler(context.Background(), &queue.Message{Type: "lock.task"})
	if !errors.Is(err, queue.ErrLockNotAcquired) {
		t.Fatalf("handler error = %v, want ErrLockNotAcquired", err)
	}
	if called {
		t.Fatal("business handler should not run when lock is not acquired")
	}
}

func TestRuntimeMiddlewareLockUnlockErrorDoesNotFailBusinessSuccess(t *testing.T) {
	wantErr := errors.New("unlock failed")
	locker := &runtimeLocker{unlockErr: wantErr}
	called := false
	handler := middleware.Lock(locker, middleware.StaticLockKey("task:1", time.Second))(
		func(context.Context, *queue.Message) error {
			called = true
			return nil
		},
	)

	if err := handler(context.Background(), &queue.Message{Type: "lock.task"}); err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if !called || !locker.unlocked {
		t.Fatalf("called=%v unlocked=%v", called, locker.unlocked)
	}
}

type runtimeLocker struct {
	conflict  bool
	locked    bool
	unlocked  bool
	unlockErr error
}

func (l *runtimeLocker) Lock(ctx context.Context, key string, ttl time.Duration) (func(context.Context) error, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if l.conflict {
		return nil, false, nil
	}
	if key != "task:1" || ttl != time.Second {
		return nil, false, errors.New("unexpected lock arguments")
	}
	l.locked = true
	return func(context.Context) error {
		l.unlocked = true
		return l.unlockErr
	}, true, nil
}
