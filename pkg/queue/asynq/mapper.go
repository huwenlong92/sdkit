package asynq

import (
	"time"

	"github.com/huwenlong92/sdkit/core/queue"

	hibasynq "github.com/hibiken/asynq"
)

func mapAsynqState(s hibasynq.TaskState) queue.TaskState {
	switch s {
	case hibasynq.TaskStatePending:
		return queue.StatePending
	case hibasynq.TaskStateActive:
		return queue.StateActive
	case hibasynq.TaskStateScheduled:
		return queue.StateScheduled
	case hibasynq.TaskStateRetry:
		return queue.StateRetry
	case hibasynq.TaskStateArchived:
		return queue.StateArchived
	case hibasynq.TaskStateCompleted:
		return queue.StateSucceeded
	default:
		return queue.StateUnknown
	}
}

func fromAsynqTaskInfo(info *hibasynq.TaskInfo) *queue.TaskInfo {
	if info == nil {
		return nil
	}
	out := &queue.TaskInfo{
		ID:        info.ID,
		Queue:     info.Queue,
		Type:      info.Type,
		State:     mapAsynqState(info.State),
		Payload:   cloneBytes(info.Payload),
		Headers:   cloneHeaders(info.Headers),
		MaxRetry:  info.MaxRetry,
		Retried:   info.Retried,
		LastError: info.LastErr,
		Timeout:   info.Timeout,
		Result:    string(info.Result),
	}
	if !info.Deadline.IsZero() {
		t := info.Deadline
		out.Deadline = &t
	}
	if !info.NextProcessAt.IsZero() {
		t := info.NextProcessAt
		out.NextRunAt = &t
	}
	if !info.CompletedAt.IsZero() {
		t := info.CompletedAt
		out.LastRunAt = &t
	}
	if out.UpdatedAt.IsZero() {
		out.UpdatedAt = time.Now()
	}
	meta := taskMetaFromHeaders(out.Headers)
	out.TrackID = meta.TrackID
	out.RequestID = meta.RequestID
	out.TraceID = meta.TraceID
	out.SpanID = meta.SpanID
	out.TenantID = meta.TenantID
	out.UserID = meta.UserID
	return out
}

func fromAsynqQueueInfo(info *hibasynq.QueueInfo) *queue.QueueInfo {
	if info == nil {
		return nil
	}
	state := queue.QueueRunning
	if info.Paused {
		state = queue.QueuePaused
	}
	return &queue.QueueInfo{
		Name:      info.Queue,
		State:     state,
		Pending:   int64(info.Pending),
		Active:    int64(info.Active),
		Scheduled: int64(info.Scheduled),
		Retry:     int64(info.Retry),
		Archived:  int64(info.Archived),
		Failed:    int64(info.Archived),
		Succeeded: int64(info.Completed),
		Processed: int64(info.ProcessedTotal),
		FailedAll: int64(info.FailedTotal),
		UpdatedAt: info.Timestamp,
	}
}

func taskMetaFromHeaders(headers map[string]string) queue.TaskMeta {
	return queue.TaskMeta{
		TrackID:   queue.TrackIDFromHeaders(headers),
		RequestID: queue.RequestIDFromHeaders(headers),
		TraceID:   queue.TraceIDFromHeaders(headers),
		SpanID:    queue.SpanIDFromHeaders(headers),
		TenantID:  queue.CorrelationHeaderValue(headers, "tenant_id"),
		UserID:    queue.CorrelationHeaderValue(headers, "user_id"),
	}
}

func cloneBytes(in []byte) []byte {
	if in == nil {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

func cloneHeaders(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
