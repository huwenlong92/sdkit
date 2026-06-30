package queue_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"
)

func TestTaskStoreMiddlewareFinishesWithDetachedContext(t *testing.T) {
	store := &taskStoreMiddlewareStore{}
	wantErr := errors.New("handler failed")
	ctx, cancel := context.WithCancel(context.Background())

	handler := queue.TaskStoreMiddleware(store, queue.TaskStoreOptions{})(
		func(context.Context, *queue.Message) error {
			cancel()
			return wantErr
		},
	)

	err := handler(ctx, &queue.Message{
		ID:       "task-1",
		Type:     "task.detached.finish",
		Queue:    queue.DefaultQueueName,
		MaxRetry: 0,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("handler error = %v, want %v", err, wantErr)
	}
	if !store.finished {
		t.Fatal("FinishRun was not called")
	}
	if store.finishCtxErr != nil {
		t.Fatalf("finish context err = %v, want nil", store.finishCtxErr)
	}
	if store.finishedStatus != queue.StateFailed {
		t.Fatalf("finished status = %s, want %s", store.finishedStatus, queue.StateFailed)
	}
}

type taskStoreMiddlewareStore struct {
	finished       bool
	finishedStatus queue.TaskState
	finishCtxErr   error
}

func (s *taskStoreMiddlewareStore) RecordEnqueued(context.Context, queue.TaskRecord) error {
	return nil
}

func (s *taskStoreMiddlewareStore) EnsureRunning(_ context.Context, record queue.TaskRecord) (queue.TaskRecord, error) {
	record.RecordID = 1
	return record, nil
}

func (s *taskStoreMiddlewareStore) StartRun(_ context.Context, run queue.TaskRunRecord) (queue.TaskRunRecord, error) {
	run.RunDBID = 1
	run.RunID = "run-1"
	return run, nil
}

func (s *taskStoreMiddlewareStore) FinishRun(ctx context.Context, run queue.TaskRunRecord) error {
	s.finished = true
	s.finishedStatus = run.Status
	s.finishCtxErr = ctx.Err()
	return nil
}

func (s *taskStoreMiddlewareStore) AppendRunLog(context.Context, queue.TaskRunLogRecord) error {
	return nil
}

func (s *taskStoreMiddlewareStore) UpdateTaskStatus(context.Context, queue.TaskStatusUpdate) error {
	return nil
}

var _ queue.TaskStore = (*taskStoreMiddlewareStore)(nil)

func TestTaskStoreMiddlewareUsesCustomFinishTimeout(t *testing.T) {
	store := &taskStoreMiddlewareStore{}
	handler := queue.TaskStoreMiddleware(store, queue.TaskStoreOptions{
		FinishTimeout: time.Second,
	})(func(context.Context, *queue.Message) error {
		return nil
	})

	if err := handler(context.Background(), &queue.Message{ID: "task-1", Type: "task.timeout"}); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !store.finished {
		t.Fatal("FinishRun was not called")
	}
}
