package realtime

import "encoding/json"

const DefaultTopic = "rt:events"

const (
	TargetUser      = "user"
	TargetRoom      = "room"
	TargetBroadcast = "broadcast"
)

type Target struct {
	Type     string `json:"type"`
	ID       string `json:"id,omitempty"`
	UserID   string `json:"user_id,omitempty"`
	TenantID int64  `json:"tenant_id,omitempty"`
	RoomID   string `json:"room_id,omitempty"`
	ClientID string `json:"client_id,omitempty"`
}

type Event struct {
	Action    string            `json:"action"`
	RequestID string            `json:"request_id,omitempty"`
	TraceID   string            `json:"trace_id,omitempty"`
	Timestamp int64             `json:"timestamp,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Data      any               `json:"data,omitempty"`

	Event    string          `json:"event,omitempty"`
	Module   string          `json:"module,omitempty"`
	Target   *Target         `json:"target,omitempty"`
	TenantID int64           `json:"tenant_id,omitempty"`
	RoomID   string          `json:"room_id,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
	Time     int64           `json:"time,omitempty"`
}

func (e *Event) Normalize() *Event {
	if e == nil {
		return nil
	}
	if e.Action == "" {
		e.Action = e.Event
	}
	if e.Event == "" {
		e.Event = e.Action
	}
	if e.Timestamp == 0 {
		e.Timestamp = e.Time
	}
	if e.Time == 0 {
		e.Time = e.Timestamp
	}
	if e.Data == nil && len(e.Payload) > 0 {
		e.Data = e.Payload
	}
	return e
}
