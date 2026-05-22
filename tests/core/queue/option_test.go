package queue_test

import (
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"
)

func TestApplyOptionsDefaults(t *testing.T) {
	opts := queue.ApplyOptions(nil)

	if opts.Queue != queue.DefaultQueueName {
		t.Fatalf("Queue = %q, want %q", opts.Queue, queue.DefaultQueueName)
	}
	if !opts.Trace {
		t.Fatalf("Trace = false, want true")
	}
	if opts.MaxRetry != nil {
		t.Fatalf("MaxRetry = %v, want nil", *opts.MaxRetry)
	}
}

func TestApplyOptionsCoversAllEnqueueOptions(t *testing.T) {
	processAt := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	deadline := processAt.Add(30 * time.Minute)

	opts := queue.ApplyOptions([]queue.Option{
		queue.Queue("critical"),
		queue.TaskID("task-1"),
		queue.MaxRetry(3),
		queue.Timeout(10 * time.Second),
		queue.Deadline(deadline),
		queue.ProcessAt(processAt),
		queue.ProcessIn(5 * time.Second),
		queue.Unique(time.Minute),
		queue.Retention(2 * time.Hour),
		queue.Group("tenant-a"),
		queue.WithPriority(9),
		queue.WithRateLimitKey("tenant-a:email"),
		queue.AutoRetry(4, 15*time.Second),
		queue.WithTrace(false),
	})

	if opts.Queue != "critical" {
		t.Fatalf("Queue = %q, want critical", opts.Queue)
	}
	if opts.TaskID != "task-1" {
		t.Fatalf("TaskID = %q, want task-1", opts.TaskID)
	}
	if opts.MaxRetry == nil || *opts.MaxRetry != 3 {
		t.Fatalf("MaxRetry = %v, want 3", opts.MaxRetry)
	}
	if opts.Timeout != 10*time.Second {
		t.Fatalf("Timeout = %s, want 10s", opts.Timeout)
	}
	if !opts.Deadline.Equal(deadline) {
		t.Fatalf("Deadline = %s, want %s", opts.Deadline, deadline)
	}
	if !opts.ProcessAt.Equal(processAt) {
		t.Fatalf("ProcessAt = %s, want %s", opts.ProcessAt, processAt)
	}
	if opts.ProcessIn != 5*time.Second {
		t.Fatalf("ProcessIn = %s, want 5s", opts.ProcessIn)
	}
	if opts.UniqueTTL != time.Minute {
		t.Fatalf("UniqueTTL = %s, want 1m", opts.UniqueTTL)
	}
	if opts.Retention != 2*time.Hour {
		t.Fatalf("Retention = %s, want 2h", opts.Retention)
	}
	if opts.Group != "tenant-a" {
		t.Fatalf("Group = %q, want tenant-a", opts.Group)
	}
	if opts.Priority != 9 {
		t.Fatalf("Priority = %d, want 9", opts.Priority)
	}
	if opts.RateLimitKey != "tenant-a:email" {
		t.Fatalf("RateLimitKey = %q, want tenant-a:email", opts.RateLimitKey)
	}
	if !opts.AutoRetryEnabled || opts.AutoRetryMax != 4 || opts.AutoRetryDelay != 15*time.Second {
		t.Fatalf("AutoRetry = enabled:%v max:%d delay:%s, want enabled true max 4 delay 15s", opts.AutoRetryEnabled, opts.AutoRetryMax, opts.AutoRetryDelay)
	}
	if opts.Trace {
		t.Fatalf("Trace = true, want false")
	}
}

func TestApplyOptionsAliases(t *testing.T) {
	processAt := time.Date(2026, 5, 22, 11, 0, 0, 0, time.UTC)
	deadline := processAt.Add(time.Hour)

	opts := queue.ApplyOptions([]queue.Option{
		queue.WithQueue("bulk"),
		queue.WithTaskID("task-alias"),
		queue.WithRetry(2),
		queue.WithMaxRetry(3),
		queue.WithTimeout(20 * time.Second),
		queue.WithDeadline(deadline),
		queue.WithProcessAt(processAt),
		queue.WithProcessIn(25 * time.Second),
		queue.WithDelay(30 * time.Second),
		queue.WithUnique(2 * time.Minute),
		queue.WithRetention(3 * time.Hour),
		queue.WithGroup("group-alias"),
		queue.WithAutoRetry(5, 40*time.Second),
	})

	if opts.Queue != "bulk" || opts.TaskID != "task-alias" {
		t.Fatalf("Queue/TaskID = %q/%q, want bulk/task-alias", opts.Queue, opts.TaskID)
	}
	if opts.MaxRetry == nil || *opts.MaxRetry != 3 {
		t.Fatalf("MaxRetry = %v, want 3", opts.MaxRetry)
	}
	if opts.Timeout != 20*time.Second || !opts.Deadline.Equal(deadline) || !opts.ProcessAt.Equal(processAt) {
		t.Fatalf("time options were not applied correctly")
	}
	if opts.ProcessIn != 30*time.Second {
		t.Fatalf("ProcessIn = %s, want WithDelay to override to 30s", opts.ProcessIn)
	}
	if opts.UniqueTTL != 2*time.Minute || opts.Retention != 3*time.Hour || opts.Group != "group-alias" {
		t.Fatalf("unique/retention/group aliases were not applied correctly")
	}
	if !opts.AutoRetryEnabled || opts.AutoRetryMax != 5 || opts.AutoRetryDelay != 40*time.Second {
		t.Fatalf("AutoRetry alias = enabled:%v max:%d delay:%s, want enabled true max 5 delay 40s", opts.AutoRetryEnabled, opts.AutoRetryMax, opts.AutoRetryDelay)
	}
}

func TestApplyOptionsNormalizesRetryValues(t *testing.T) {
	maxRetry := queue.ApplyOptions([]queue.Option{queue.MaxRetry(-1)})
	if maxRetry.MaxRetry == nil || *maxRetry.MaxRetry != 0 {
		t.Fatalf("MaxRetry(-1) = %v, want 0", maxRetry.MaxRetry)
	}

	autoRetry := queue.ApplyOptions([]queue.Option{queue.AutoRetry(2, -time.Second)})
	if !autoRetry.AutoRetryEnabled || autoRetry.AutoRetryMax != 2 || autoRetry.AutoRetryDelay != 0 {
		t.Fatalf("AutoRetry negative delay = enabled:%v max:%d delay:%s, want enabled true max 2 delay 0", autoRetry.AutoRetryEnabled, autoRetry.AutoRetryMax, autoRetry.AutoRetryDelay)
	}

	disabled := queue.ApplyOptions([]queue.Option{queue.AutoRetry(0, time.Minute)})
	if disabled.AutoRetryEnabled || disabled.AutoRetryMax != 0 || disabled.AutoRetryDelay != 0 {
		t.Fatalf("AutoRetry(0) = enabled:%v max:%d delay:%s, want disabled", disabled.AutoRetryEnabled, disabled.AutoRetryMax, disabled.AutoRetryDelay)
	}
}

func TestApplyOptionsIgnoresNilOption(t *testing.T) {
	opts := queue.ApplyOptions([]queue.Option{
		nil,
		queue.Queue("default"),
	})

	if opts.Queue != "default" {
		t.Fatalf("Queue = %q, want default", opts.Queue)
	}
}
