package reporter

import (
	"context"
	"strings"
	"time"
)

const (
	DefaultProgressRunningTTL  = 6 * time.Hour
	DefaultProgressTerminalTTL = 2 * time.Hour
	DefaultProgressRecentLimit = 50
)

type ProgressStep struct {
	Key       string         `json:"key"`
	Step      string         `json:"step,omitempty"`
	Status    string         `json:"status,omitempty"`
	Percent   int32          `json:"percent,omitempty"`
	Message   string         `json:"message,omitempty"`
	Detail    map[string]any `json:"detail,omitempty"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type ProgressSnapshot struct {
	OperationID   string         `json:"operation_id"`
	OperationType string         `json:"operation_type,omitempty"`
	Domain        string         `json:"domain,omitempty"`
	RefID         string         `json:"ref_id,omitempty"`
	Subject       map[string]any `json:"subject,omitempty"`
	Step          string         `json:"step,omitempty"`
	Status        string         `json:"status,omitempty"`
	Percent       int32          `json:"percent,omitempty"`
	Message       string         `json:"message,omitempty"`
	Terminal      bool           `json:"terminal"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Steps         []ProgressStep `json:"steps,omitempty"`
	Recent        []Event        `json:"recent,omitempty"`
}

type ProgressStore interface {
	Save(ctx context.Context, key string, snapshot ProgressSnapshot, ttl time.Duration) error
	Load(ctx context.Context, key string) (ProgressSnapshot, bool, error)
	Delete(ctx context.Context, key string) error
}

type ProgressKeyFunc func(Event) string

type ProgressSnapshotSink struct {
	Store       ProgressStore
	KeyFunc     ProgressKeyFunc
	RunningTTL  time.Duration
	TerminalTTL time.Duration
	RecentLimit int
}

type ProgressCleanupSink struct {
	Store   ProgressStore
	KeyFunc ProgressKeyFunc
}

type ProgressSnapshotOption func(*ProgressSnapshotSink)

type ProgressCleanupOption func(*ProgressCleanupSink)

func NewProgressSnapshotSink(store ProgressStore, options ...ProgressSnapshotOption) ProgressSnapshotSink {
	sink := ProgressSnapshotSink{
		Store:       store,
		KeyFunc:     func(event Event) string { return event.SnapshotKey() },
		RunningTTL:  DefaultProgressRunningTTL,
		TerminalTTL: DefaultProgressTerminalTTL,
		RecentLimit: DefaultProgressRecentLimit,
	}
	for _, option := range options {
		if option != nil {
			option(&sink)
		}
	}
	return sink
}

func WithProgressKeyFunc(fn ProgressKeyFunc) ProgressSnapshotOption {
	return func(s *ProgressSnapshotSink) {
		if fn != nil {
			s.KeyFunc = fn
		}
	}
}

func WithProgressTTL(running time.Duration, terminal time.Duration) ProgressSnapshotOption {
	return func(s *ProgressSnapshotSink) {
		s.RunningTTL = running
		s.TerminalTTL = terminal
	}
}

func WithProgressRecentLimit(limit int) ProgressSnapshotOption {
	return func(s *ProgressSnapshotSink) {
		s.RecentLimit = limit
	}
}

func NewProgressCleanupSink(store ProgressStore, options ...ProgressCleanupOption) ProgressCleanupSink {
	sink := ProgressCleanupSink{
		Store:   store,
		KeyFunc: func(event Event) string { return event.SnapshotKey() },
	}
	for _, option := range options {
		if option != nil {
			option(&sink)
		}
	}
	return sink
}

func WithProgressCleanupKeyFunc(fn ProgressKeyFunc) ProgressCleanupOption {
	return func(s *ProgressCleanupSink) {
		if fn != nil {
			s.KeyFunc = fn
		}
	}
}

func (s ProgressSnapshotSink) Handle(ctx context.Context, event Event) error {
	if s.Store == nil {
		return nil
	}
	keyFunc := s.KeyFunc
	if keyFunc == nil {
		keyFunc = func(event Event) string { return event.SnapshotKey() }
	}
	key := strings.TrimSpace(keyFunc(event))
	if key == "" {
		return nil
	}
	snapshot, ok, err := s.Store.Load(ctx, key)
	if err != nil {
		return err
	}
	if !ok {
		snapshot = ProgressSnapshot{OperationID: event.OperationID}
	}
	snapshot.Apply(event, s.RecentLimit)
	ttl := s.RunningTTL
	if snapshot.Terminal {
		ttl = s.TerminalTTL
	}
	return s.Store.Save(ctx, key, snapshot, ttl)
}

func (s ProgressCleanupSink) Handle(ctx context.Context, event Event) error {
	if s.Store == nil || !event.IsTerminal() {
		return nil
	}
	keyFunc := s.KeyFunc
	if keyFunc == nil {
		keyFunc = func(event Event) string { return event.SnapshotKey() }
	}
	key := strings.TrimSpace(keyFunc(event))
	if key == "" {
		return nil
	}
	return s.Store.Delete(ctx, key)
}

func (s *ProgressSnapshot) Apply(event Event, recentLimit int) {
	event = event.Clone()
	if event.At.IsZero() {
		event.At = now()
	}
	if event.OperationID != "" {
		s.OperationID = event.OperationID
	}
	if event.OperationType != "" {
		s.OperationType = event.OperationType
	}
	if event.Domain != "" {
		s.Domain = event.Domain
	}
	if event.RefID != "" {
		s.RefID = event.RefID
	}
	if len(event.Subject) > 0 {
		s.Subject = cloneMap(event.Subject)
	}
	if event.Step != "" {
		s.Step = event.Step
	}
	if event.Status != "" {
		s.Status = event.Status
	}
	if event.Message != "" {
		s.Message = event.Message
	}
	if event.Percent >= 0 {
		s.Percent = event.Percent
	}
	s.UpdatedAt = event.At
	if event.IsTerminal() {
		s.Terminal = true
	}
	if key := event.StepKey(); key != "" {
		s.applyStep(key, event)
	}
	if recentLimit > 0 {
		s.Recent = append(s.Recent, event)
		if len(s.Recent) > recentLimit {
			s.Recent = append([]Event(nil), s.Recent[len(s.Recent)-recentLimit:]...)
		}
	}
}

func (s *ProgressSnapshot) applyStep(key string, event Event) {
	step := ProgressStep{
		Key:       key,
		Step:      event.Step,
		Status:    event.Status,
		Percent:   event.Percent,
		Message:   event.Message,
		Detail:    cloneMap(event.Detail),
		UpdatedAt: event.At,
	}
	for i := range s.Steps {
		if s.Steps[i].Key == key {
			s.Steps[i] = step
			return
		}
	}
	s.Steps = append(s.Steps, step)
}
