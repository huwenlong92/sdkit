package crontab

import (
	"sync/atomic"
	"time"
)

type RuntimeMetrics struct {
	CrontabExecuteTotal        int64         `json:"crontab_execute_total"`
	CrontabExecuteSuccessTotal int64         `json:"crontab_execute_success_total"`
	CrontabExecuteFailedTotal  int64         `json:"crontab_execute_failed_total"`
	CrontabExecuteDuration     time.Duration `json:"crontab_execute_duration"`
	CrontabOverlapSkippedTotal int64         `json:"crontab_overlap_skipped_total"`
	CrontabTimeoutTotal        int64         `json:"crontab_timeout_total"`
}

var runtimeMetricCounters struct {
	executeTotal   atomic.Int64
	successTotal   atomic.Int64
	failedTotal    atomic.Int64
	durationNanos  atomic.Int64
	overlapSkipped atomic.Int64
	timeoutTotal   atomic.Int64
}

func RuntimeMetricsSnapshot() RuntimeMetrics {
	return RuntimeMetrics{
		CrontabExecuteTotal:        runtimeMetricCounters.executeTotal.Load(),
		CrontabExecuteSuccessTotal: runtimeMetricCounters.successTotal.Load(),
		CrontabExecuteFailedTotal:  runtimeMetricCounters.failedTotal.Load(),
		CrontabExecuteDuration:     time.Duration(runtimeMetricCounters.durationNanos.Load()),
		CrontabOverlapSkippedTotal: runtimeMetricCounters.overlapSkipped.Load(),
		CrontabTimeoutTotal:        runtimeMetricCounters.timeoutTotal.Load(),
	}
}

func recordRuntimeExecution(status Status, duration time.Duration) {
	runtimeMetricCounters.executeTotal.Add(1)
	runtimeMetricCounters.durationNanos.Add(duration.Nanoseconds())
	if status == StatusSuccess {
		runtimeMetricCounters.successTotal.Add(1)
		return
	}
	switch status {
	case StatusLocked:
		runtimeMetricCounters.overlapSkipped.Add(1)
	case StatusTimeout:
		runtimeMetricCounters.failedTotal.Add(1)
		runtimeMetricCounters.timeoutTotal.Add(1)
	case StatusFailed, StatusPanic:
		runtimeMetricCounters.failedTotal.Add(1)
	}
}

func resetRuntimeMetrics() {
	runtimeMetricCounters.executeTotal.Store(0)
	runtimeMetricCounters.successTotal.Store(0)
	runtimeMetricCounters.failedTotal.Store(0)
	runtimeMetricCounters.durationNanos.Store(0)
	runtimeMetricCounters.overlapSkipped.Store(0)
	runtimeMetricCounters.timeoutTotal.Store(0)
}
