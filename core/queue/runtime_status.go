package queue

import "time"

type RuntimeState string

const (
	RuntimeRunning  RuntimeState = "running"
	RuntimePaused   RuntimeState = "paused"
	RuntimeDraining RuntimeState = "draining"
	RuntimeStopped  RuntimeState = "stopped"
	RuntimeFailed   RuntimeState = "failed"
)

type QueueRuntimeStatus struct {
	Name      string
	State     QueueState
	Priority  int
	Metrics   QueueMetrics
	PausedAt  *time.Time
	UpdatedAt time.Time
}

type WorkerRuntimeStatus struct {
	ID             string
	Name           string
	Service        string
	State          RuntimeState
	Queues         map[string]int
	StrictPriority bool
	Concurrency    int
	UpdatedAt      time.Time
}

type RuntimeStatusInfo struct {
	State     RuntimeState
	Queues    []QueueRuntimeStatus
	Worker    WorkerRuntimeStatus
	UpdatedAt time.Time
	Error     string
}

type QueueMetrics struct {
	Pending   int64
	Active    int64
	Scheduled int64
	Retry     int64
	Archived  int64
	Succeeded int64
	Failed    int64
	Canceled  int64
	Processed int64
	FailedAll int64
}

type RuntimeMetrics struct {
	Queues map[string]QueueMetrics
	Total  QueueMetrics
}

func queueMetricsFromInfo(info *QueueInfo) QueueMetrics {
	if info == nil {
		return QueueMetrics{}
	}
	return QueueMetrics{
		Pending:   info.Pending,
		Active:    info.Active,
		Scheduled: info.Scheduled,
		Retry:     info.Retry,
		Archived:  info.Archived,
		Succeeded: info.Succeeded,
		Failed:    info.Failed,
		Canceled:  info.Canceled,
		Processed: info.Processed,
		FailedAll: info.FailedAll,
	}
}

func addQueueMetrics(total QueueMetrics, item QueueMetrics) QueueMetrics {
	total.Pending += item.Pending
	total.Active += item.Active
	total.Scheduled += item.Scheduled
	total.Retry += item.Retry
	total.Archived += item.Archived
	total.Succeeded += item.Succeeded
	total.Failed += item.Failed
	total.Canceled += item.Canceled
	total.Processed += item.Processed
	total.FailedAll += item.FailedAll
	return total
}
