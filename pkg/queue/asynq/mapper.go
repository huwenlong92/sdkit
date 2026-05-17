package asynq

import (
	"time"

	corequeue "github.com/huwenlong92/sdkit/core/queue"

	hibasynq "github.com/hibiken/asynq"
)

func mapAsynqState(s hibasynq.TaskState) corequeue.TaskState {
	switch s {
	case hibasynq.TaskStatePending:
		return corequeue.StatePending
	case hibasynq.TaskStateActive:
		return corequeue.StateActive
	case hibasynq.TaskStateScheduled:
		return corequeue.StateScheduled
	case hibasynq.TaskStateRetry:
		return corequeue.StateRetry
	case hibasynq.TaskStateArchived:
		return corequeue.StateArchived
	case hibasynq.TaskStateCompleted:
		return corequeue.StateSucceeded
	default:
		return corequeue.StateUnknown
	}
}

func fromAsynqTaskInfo(info *hibasynq.TaskInfo) *corequeue.TaskInfo {
	if info == nil {
		return nil
	}
	out := &corequeue.TaskInfo{
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

func fromAsynqQueueInfo(info *hibasynq.QueueInfo) *corequeue.QueueInfo {
	if info == nil {
		return nil
	}
	state := corequeue.QueueRunning
	if info.Paused {
		state = corequeue.QueuePaused
	}
	return &corequeue.QueueInfo{
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

func taskMetaFromHeaders(headers map[string]string) corequeue.TaskMeta {
	return corequeue.TaskMeta{
		TrackID:   corequeue.TrackIDFromHeaders(headers),
		RequestID: corequeue.RequestIDFromHeaders(headers),
		TraceID:   corequeue.TraceIDFromHeaders(headers),
		SpanID:    corequeue.SpanIDFromHeaders(headers),
		TenantID:  corequeue.CorrelationHeaderValue(headers, "tenant_id"),
		UserID:    corequeue.CorrelationHeaderValue(headers, "user_id"),
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
