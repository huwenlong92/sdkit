package gormoutbox

import (
	"testing"
	"time"

	corequeue "github.com/huwenlong92/sdkit/core/queue"
)

func TestOutboxOptionsRoundTripForSupportedDrivers(t *testing.T) {
	processAt := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	deadline := processAt.Add(30 * time.Minute)

	for _, driver := range []string{"asynq", "nats"} {
		t.Run(driver, func(t *testing.T) {
			applied := corequeue.ApplyOptions([]corequeue.Option{
				corequeue.Queue(driver + "-queue"),
				corequeue.TaskID(driver + "-task"),
				corequeue.MaxRetry(3),
				corequeue.Timeout(10 * time.Second),
				corequeue.Deadline(deadline),
				corequeue.ProcessAt(processAt),
				corequeue.ProcessIn(5 * time.Second),
				corequeue.Unique(time.Minute),
				corequeue.Retention(2 * time.Hour),
				corequeue.Group(driver + "-group"),
				corequeue.WithPriority(7),
				corequeue.WithRateLimitKey(driver + ":tenant"),
				corequeue.AutoRetry(4, 15*time.Second),
				corequeue.WithTrace(false),
			})

			encoded := toOutboxOptions(applied)
			decoded := corequeue.ApplyOptions(fromOutboxOptions(encoded))

			assertEnqueueOptionsEqual(t, decoded, applied)
		})
	}
}

func TestOutboxOptionsDefaultTraceRoundTrip(t *testing.T) {
	applied := corequeue.ApplyOptions([]corequeue.Option{corequeue.Queue("default")})
	decoded := corequeue.ApplyOptions(fromOutboxOptions(toOutboxOptions(applied)))

	if !decoded.Trace {
		t.Fatalf("Trace = false, want true")
	}
}

func assertEnqueueOptionsEqual(t *testing.T, got corequeue.EnqueueOptions, want corequeue.EnqueueOptions) {
	t.Helper()

	if got.Queue != want.Queue {
		t.Fatalf("Queue = %q, want %q", got.Queue, want.Queue)
	}
	if got.TaskID != want.TaskID {
		t.Fatalf("TaskID = %q, want %q", got.TaskID, want.TaskID)
	}
	if (got.MaxRetry == nil) != (want.MaxRetry == nil) {
		t.Fatalf("MaxRetry nil = %v, want %v", got.MaxRetry == nil, want.MaxRetry == nil)
	}
	if got.MaxRetry != nil && *got.MaxRetry != *want.MaxRetry {
		t.Fatalf("MaxRetry = %d, want %d", *got.MaxRetry, *want.MaxRetry)
	}
	if got.Timeout != want.Timeout {
		t.Fatalf("Timeout = %s, want %s", got.Timeout, want.Timeout)
	}
	if !got.Deadline.Equal(want.Deadline) {
		t.Fatalf("Deadline = %s, want %s", got.Deadline, want.Deadline)
	}
	if !got.ProcessAt.Equal(want.ProcessAt) {
		t.Fatalf("ProcessAt = %s, want %s", got.ProcessAt, want.ProcessAt)
	}
	if got.ProcessIn != want.ProcessIn {
		t.Fatalf("ProcessIn = %s, want %s", got.ProcessIn, want.ProcessIn)
	}
	if got.UniqueTTL != want.UniqueTTL {
		t.Fatalf("UniqueTTL = %s, want %s", got.UniqueTTL, want.UniqueTTL)
	}
	if got.Retention != want.Retention {
		t.Fatalf("Retention = %s, want %s", got.Retention, want.Retention)
	}
	if got.Group != want.Group {
		t.Fatalf("Group = %q, want %q", got.Group, want.Group)
	}
	if got.Priority != want.Priority {
		t.Fatalf("Priority = %d, want %d", got.Priority, want.Priority)
	}
	if got.RateLimitKey != want.RateLimitKey {
		t.Fatalf("RateLimitKey = %q, want %q", got.RateLimitKey, want.RateLimitKey)
	}
	if got.Trace != want.Trace {
		t.Fatalf("Trace = %v, want %v", got.Trace, want.Trace)
	}
	if got.AutoRetryEnabled != want.AutoRetryEnabled || got.AutoRetryMax != want.AutoRetryMax || got.AutoRetryDelay != want.AutoRetryDelay {
		t.Fatalf("AutoRetry = enabled:%v max:%d delay:%s, want enabled:%v max:%d delay:%s",
			got.AutoRetryEnabled, got.AutoRetryMax, got.AutoRetryDelay,
			want.AutoRetryEnabled, want.AutoRetryMax, want.AutoRetryDelay)
	}
}
