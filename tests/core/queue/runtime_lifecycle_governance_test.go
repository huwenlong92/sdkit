package queue_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/queue/runtime/middleware"
)

func TestDispatcherLifecycleHooksWrapMiddlewareChain(t *testing.T) {
	dispatcher := queue.NewDispatcher()
	order := make([]string, 0, 6)
	dispatcher.AddHook(queue.HookFunc{
		Before: func(context.Context, *queue.Message) error {
			order = append(order, "before")
			return nil
		},
		After: func(_ context.Context, _ *queue.Message, err error) {
			if err != nil {
				t.Fatalf("after err = %v", err)
			}
			order = append(order, "after")
		},
		Success: func(context.Context, *queue.Message) {
			order = append(order, "success")
		},
		Failure: func(context.Context, *queue.Message, error) {
			t.Fatal("failure hook should not run")
		},
	})
	dispatcher.Use(func(next queue.HandlerFunc) queue.HandlerFunc {
		return func(ctx context.Context, msg *queue.Message) error {
			order = append(order, "middleware")
			return next(ctx, msg)
		}
	})

	if err := dispatcher.Register("task.lifecycle", func(context.Context, *queue.Message) error {
		order = append(order, "handler")
		return nil
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := dispatcher.Dispatch(context.Background(), "task.lifecycle", nil); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	want := []string{"before", "middleware", "handler", "after", "success"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestDispatcherLifecycleHookFailureShortCircuitsHandler(t *testing.T) {
	dispatcher := queue.NewDispatcher()
	wantErr := errors.New("before failed")
	calledHandler := false
	calledAfter := false
	calledFailure := false
	dispatcher.AddHook(queue.HookFunc{
		Before: func(context.Context, *queue.Message) error {
			return wantErr
		},
		After: func(_ context.Context, _ *queue.Message, err error) {
			calledAfter = errors.Is(err, wantErr)
		},
		Failure: func(_ context.Context, _ *queue.Message, err error) {
			calledFailure = errors.Is(err, wantErr)
		},
	})
	if err := dispatcher.Register("task.lifecycle", func(context.Context, *queue.Message) error {
		calledHandler = true
		return nil
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	err := dispatcher.Dispatch(context.Background(), "task.lifecycle", nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("dispatch error = %v, want %v", err, wantErr)
	}
	if calledHandler || !calledAfter || !calledFailure {
		t.Fatalf("handler=%v after=%v failure=%v", calledHandler, calledAfter, calledFailure)
	}
}

func TestRuntimeGovernanceMiddlewares(t *testing.T) {
	metrics := &runtimeMetricsRecorder{}
	limiter := &runtimeConcurrencyLimiter{}
	deadletter := &runtimeDeadLetter{}
	wantErr := errors.New("boom")

	handler := queue.Chain(
		func(context.Context, *queue.Message) error {
			return wantErr
		},
		middleware.Metrics(metrics),
		middleware.Concurrency(limiter, middleware.StaticConcurrencyKey("sandbox")),
		middleware.Retry(queue.RetryStrategyFunc(func(context.Context, *queue.Message, int, error) (time.Duration, bool) {
			return 3 * time.Second, true
		})),
		middleware.DeadLetter(deadletter),
	)

	err := handler(context.Background(), &queue.Message{
		Type:       "sandbox.run",
		Queue:      "critical",
		RetryCount: 2,
		MaxRetry:   2,
	})
	if !queue.IsDeadLetterError(err) {
		t.Fatalf("handler error = %v, want deadletter runtime error", err)
	}
	runtimeErr, _ := queue.RuntimeErrorFrom(err)
	if runtimeErr.Kind != queue.ErrorDeadLetter {
		t.Fatalf("runtime error kind = %s, want %s", runtimeErr.Kind, queue.ErrorDeadLetter)
	}
	if limiter.acquired != "sandbox" || limiter.released != "sandbox" {
		t.Fatalf("limiter acquired=%q released=%q", limiter.acquired, limiter.released)
	}
	if deadletter.count != 1 {
		t.Fatalf("deadletter count = %d, want 1", deadletter.count)
	}
	if metrics.counter[middleware.MetricQueueTaskTotal] != 1 ||
		metrics.counter[middleware.MetricQueueTaskFailTotal] != 1 ||
		metrics.counter[middleware.MetricQueueTaskRetryTotal] != 2 ||
		metrics.duration[middleware.MetricQueueTaskDuration] == 0 {
		t.Fatalf("metrics counter=%v duration=%v", metrics.counter, metrics.duration)
	}
}

type runtimeMetricsRecorder struct {
	counter  map[string]int64
	duration map[string]time.Duration
}

func (r *runtimeMetricsRecorder) IncCounter(_ context.Context, name string, _ queue.MetricsLabels, value int64) {
	if r.counter == nil {
		r.counter = map[string]int64{}
	}
	r.counter[name] += value
}

func (r *runtimeMetricsRecorder) ObserveDuration(_ context.Context, name string, _ queue.MetricsLabels, value time.Duration) {
	if r.duration == nil {
		r.duration = map[string]time.Duration{}
	}
	r.duration[name] += value
}

type runtimeConcurrencyLimiter struct {
	acquired string
	released string
}

func (l *runtimeConcurrencyLimiter) Acquire(_ context.Context, key string) error {
	l.acquired = key
	return nil
}

func (l *runtimeConcurrencyLimiter) Release(_ context.Context, key string) {
	l.released = key
}

type runtimeDeadLetter struct {
	count int
}

func (d *runtimeDeadLetter) Push(context.Context, *queue.Message, error) error {
	d.count++
	return nil
}
