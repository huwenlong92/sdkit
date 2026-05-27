//go:build sdkit_queue_asynq

package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"
	"github.com/huwenlong92/sdkit/pkg/queue/asynq"

	goredis "github.com/redis/go-redis/v9"
)

func TestIntegrationManagerRetryArchiveRequeuePreserveHeaders(t *testing.T) {
	cfg := integrationQueueConfig(t)
	queueName := integrationQueueName()
	cfg.Queues = map[string]int{queueName: 1}
	cleanupAsynqQueue(t, cfg, queueName)

	driver := asynq.New(cfg)
	t.Cleanup(func() { _ = driver.Close() })
	taskType := "headers:retry"
	driver.Handle(taskType, func(context.Context, *queue.Message) error {
		return errors.New("force retry for header preservation test")
	})

	runCtx, cancelRun := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- driver.Run(runCtx)
	}()
	t.Cleanup(func() {
		cancelRun()
		select {
		case <-runErrCh:
		case <-time.After(5 * time.Second):
			t.Log("asynq runner did not stop before cleanup timeout")
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	trackID := "track-asynq-manager"
	requestID := "request-asynq-manager"
	enqueueCtx := tracking.WithTrackID(ctx, trackID)
	enqueueCtx = requestid.WithRequestID(enqueueCtx, requestID)
	enqueueCtx, parent := tracing.StartSpan(enqueueCtx, "http.request")
	traceID := parent.TraceID()
	taskID := fmt.Sprintf("headers-%d", time.Now().UnixNano())

	_, err := driver.Enqueue(enqueueCtx,
		queue.NewTask(taskType, map[string]string{"source": "manager-headers"}),
		queue.Queue(queueName),
		queue.TaskID(taskID),
		queue.MaxRetry(1),
	)
	parent.End()
	if err != nil {
		t.Fatalf("enqueue task: %v", err)
	}

	retryInfo := waitTaskState(t, ctx, driver, queueName, taskID, queue.StateRetry)
	assertCorrelationHeaders(t, retryInfo, trackID, requestID, traceID)

	cancelRun()
	select {
	case err := <-runErrCh:
		if err != nil {
			t.Fatalf("asynq runner: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("asynq runner did not stop")
	}

	if err := driver.ArchiveTask(ctx, queueName, taskID); err != nil {
		t.Fatalf("archive retry task: %v", err)
	}
	archivedInfo := waitTaskState(t, ctx, driver, queueName, taskID, queue.StateArchived)
	assertCorrelationHeaders(t, archivedInfo, trackID, requestID, traceID)

	if err := driver.RetryTask(ctx, queueName, taskID); err != nil {
		t.Fatalf("retry archived task: %v", err)
	}
	pendingInfo := waitTaskState(t, ctx, driver, queueName, taskID, queue.StatePending)
	assertCorrelationHeaders(t, pendingInfo, trackID, requestID, traceID)
}

func integrationQueueConfig(t *testing.T) queue.Config {
	t.Helper()
	if os.Getenv("SDKITGO_INTEGRATION") != "1" {
		t.Skip("set SDKITGO_INTEGRATION=1 to run real Redis queue integration test")
	}
	addr := os.Getenv("SDKITGO_REDIS_ADDR")
	if addr == "" {
		t.Skip("set SDKITGO_REDIS_ADDR to run real Redis queue integration test")
	}
	db := 0
	if raw := os.Getenv("SDKITGO_REDIS_DB"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("SDKITGO_REDIS_DB: %v", err)
		}
		db = n
	}
	cfg := queue.Config{
		Addr:        addr,
		Password:    os.Getenv("SDKITGO_REDIS_PASSWORD"),
		DB:          db,
		Concurrency: 1,
	}
	client := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Addr,
		Username: os.Getenv("SDKITGO_REDIS_USERNAME"),
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Fatalf("redis ping: %v", err)
	}
	_ = client.Close()
	return cfg
}

func integrationQueueName() string {
	return fmt.Sprintf("headers_%d", time.Now().UnixNano())
}

func waitTaskState(t *testing.T, ctx context.Context, driver *asynq.Queue, queueName, taskID string, state queue.TaskState) *queue.TaskInfo {
	t.Helper()
	for {
		tasks, err := driver.ListTasks(ctx, queue.TaskQuery{
			Queue:  queueName,
			State:  state,
			TaskID: taskID,
			Limit:  10,
		})
		if err != nil {
			t.Fatalf("list %s tasks: %v", state, err)
		}
		if len(tasks) == 1 {
			return tasks[0]
		}
		if err := ctx.Err(); err != nil {
			t.Fatalf("wait task %s in state %s: %v", taskID, state, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func assertCorrelationHeaders(t *testing.T, info *queue.TaskInfo, trackID, requestID, traceID string) {
	t.Helper()
	if info == nil {
		t.Fatal("task info is nil")
	}
	if got := queue.CorrelationHeaderValue(info.Headers, tracking.Header); got != trackID {
		t.Fatalf("%s header = %q, want %q in state %s", tracking.Header, got, trackID, info.State)
	}
	if got := queue.CorrelationHeaderValue(info.Headers, requestid.Header); got != requestID {
		t.Fatalf("%s header = %q, want %q in state %s", requestid.Header, got, requestID, info.State)
	}
	if got := queue.TraceIDFromHeaders(info.Headers); got != traceID {
		t.Fatalf("trace id from headers = %q, want %q in state %s", got, traceID, info.State)
	}
	if info.TrackID != trackID {
		t.Fatalf("task track_id = %q, want %q in state %s", info.TrackID, trackID, info.State)
	}
	if info.RequestID != requestID {
		t.Fatalf("task request_id = %q, want %q in state %s", info.RequestID, requestID, info.State)
	}
	if info.TraceID != traceID {
		t.Fatalf("task trace_id = %q, want %q in state %s", info.TraceID, traceID, info.State)
	}
}

func cleanupAsynqQueue(t *testing.T, cfg queue.Config, queueName string) {
	t.Helper()
	t.Cleanup(func() {
		client := goredis.NewClient(&goredis.Options{
			Addr:     cfg.Addr,
			Username: os.Getenv("SDKITGO_REDIS_USERNAME"),
			Password: cfg.Password,
			DB:       cfg.DB,
		})
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pattern := fmt.Sprintf("asynq:{%s}:*", queueName)
		iter := client.Scan(ctx, 0, pattern, 0).Iterator()
		keys := make([]string, 0)
		for iter.Next(ctx) {
			keys = append(keys, iter.Val())
		}
		if err := iter.Err(); err != nil {
			t.Logf("scan %s: %v", pattern, err)
			return
		}
		if len(keys) > 0 {
			if err := client.Del(ctx, keys...).Err(); err != nil {
				t.Logf("cleanup queue keys: %v", err)
			}
		}
		if err := client.SRem(ctx, "asynq:queues", queueName).Err(); err != nil {
			t.Logf("cleanup queue registry: %v", err)
		}
	})
}
