package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/huwenlong92/sdkit/core/jsonx"
)

type Task struct {
	ID      string
	Type    string
	Queue   string
	Payload any
	Headers map[string]string
}

type TaskMeta struct {
	TrackID   string `json:"track_id"`
	RequestID string `json:"request_id"`
	TraceID   string `json:"trace_id"`
	SpanID    string `json:"span_id"`
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
}

type TaskPayload struct {
	Version int             `json:"version"`
	Data    json.RawMessage `json:"data"`
}

type Message struct {
	ID         string
	Type       string
	Payload    []byte
	Queue      string
	State      TaskState
	RetryCount int
	MaxRetry   int
	Headers    map[string]string
	Metadata   map[string]any
	Runtime    *RuntimeContext
}

type TaskInfo struct {
	ID      string
	Type    string
	Queue   string
	State   TaskState
	Payload []byte
	Headers map[string]string

	MaxRetry  int
	Retried   int
	LastError string

	Timeout   time.Duration
	Deadline  *time.Time
	NextRunAt *time.Time
	LastRunAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time

	TrackID   string
	RequestID string
	TraceID   string
	SpanID    string
	TenantID  string
	UserID    string

	Result string
}

type QueueInfo struct {
	Name     string
	State    QueueState
	Priority int

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

	PausedAt  *time.Time
	UpdatedAt time.Time
}

type TaskQuery struct {
	Queue  string
	State  TaskState
	Type   string
	TaskID string

	Limit  int
	Offset int
	Cursor string
}

func NewTask(taskType string, payload any) Task {
	return Task{Type: taskType, Payload: payload}
}

func MarshalPayload(payload any) ([]byte, error) {
	switch v := payload.(type) {
	case nil:
		return nil, nil
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		b, err := jsonx.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
		}
		return b, nil
	}
}

func DecodePayload[T any](msg *Message) (T, error) {
	var dst T
	if msg == nil {
		return dst, errors.New("queue: nil message")
	}
	if err := jsonx.Unmarshal(msg.Payload, &dst); err != nil {
		return dst, err
	}
	return dst, nil
}
