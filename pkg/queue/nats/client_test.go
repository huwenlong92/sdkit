package nats

import (
	"errors"
	"testing"
	"time"

	corequeue "github.com/huwenlong92/sdkit/core/queue"
)

func TestNATSOptionsAcceptSupportedDeliveryOptions(t *testing.T) {
	err := validateOptions(corequeue.ApplyOptions([]corequeue.Option{
		corequeue.Queue("default"),
		corequeue.TaskID("task-1"),
		corequeue.MaxRetry(3),
		corequeue.AutoRetry(2, time.Minute),
		corequeue.WithTrace(false),
	}))
	if err != nil {
		t.Fatalf("validateOptions() error = %v, want nil", err)
	}
}

func TestNATSUnsupportedOptions(t *testing.T) {
	processAt := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name string
		opts []corequeue.Option
	}{
		{name: "timeout", opts: []corequeue.Option{corequeue.Timeout(time.Second)}},
		{name: "deadline", opts: []corequeue.Option{corequeue.Deadline(processAt)}},
		{name: "process in", opts: []corequeue.Option{corequeue.ProcessIn(time.Second)}},
		{name: "process at", opts: []corequeue.Option{corequeue.ProcessAt(processAt)}},
		{name: "unique", opts: []corequeue.Option{corequeue.Unique(time.Minute)}},
		{name: "priority", opts: []corequeue.Option{corequeue.WithPriority(1)}},
		{name: "rate limit key", opts: []corequeue.Option{corequeue.WithRateLimitKey("tenant-a")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOptions(corequeue.ApplyOptions(tc.opts))
			if !errors.Is(err, corequeue.ErrCapabilityUnsupported) {
				t.Fatalf("validateOptions() error = %v, want ErrCapabilityUnsupported", err)
			}
		})
	}
}
