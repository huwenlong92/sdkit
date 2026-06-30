package reporter

import (
	"fmt"
	"strings"
	"time"
)

type Kind string

const (
	KindLog      Kind = "log"
	KindProgress Kind = "progress"
	KindDone     Kind = "done"
	KindFailed   Kind = "failed"
	KindCanceled Kind = "canceled"
)

type Event struct {
	Kind          Kind           `json:"kind"`
	Domain        string         `json:"domain,omitempty"`
	OperationID   string         `json:"operation_id,omitempty"`
	OperationType string         `json:"operation_type,omitempty"`
	RefID         string         `json:"ref_id,omitempty"`
	Subject       map[string]any `json:"subject,omitempty"`
	Step          string         `json:"step,omitempty"`
	ProgressKey   string         `json:"progress_key,omitempty"`
	Level         string         `json:"level,omitempty"`
	Status        string         `json:"status,omitempty"`
	Percent       int32          `json:"percent,omitempty"`
	Message       string         `json:"message,omitempty"`
	Detail        map[string]any `json:"detail,omitempty"`
	At            time.Time      `json:"at"`
}

func (e Event) WithKind(kind Kind) Event {
	e.Kind = kind
	if e.At.IsZero() {
		e.At = now()
	}
	return e
}

func (e Event) Clone() Event {
	e.Subject = cloneMap(e.Subject)
	e.Detail = cloneMap(e.Detail)
	return e
}

func (e Event) IsTerminal() bool {
	switch e.Kind {
	case KindDone, KindFailed, KindCanceled:
		return true
	default:
		return false
	}
}

func (e Event) SnapshotKey() string {
	return strings.TrimSpace(e.OperationID)
}

func (e Event) StepKey() string {
	key := strings.TrimSpace(e.ProgressKey)
	if key != "" {
		return key
	}
	if e.Detail != nil {
		if raw, ok := e.Detail["progress_key"]; ok {
			if value := strings.TrimSpace(fmt.Sprint(raw)); value != "" {
				return value
			}
		}
	}
	return ""
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
