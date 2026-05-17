package crontab

import (
	"context"
	"sync"
	"time"
)

type LogLevel string

const (
	LogInfo  LogLevel = "info"
	LogWarn  LogLevel = "warn"
	LogError LogLevel = "error"
)

type LogEvent struct {
	RunID       string         `json:"run_id"`
	EntryID     string         `json:"entry_id"`
	TemplateKey string         `json:"template_key"`
	TrackID     string         `json:"track_id"`
	Level       LogLevel       `json:"level"`
	Message     string         `json:"message"`
	Fields      map[string]any `json:"fields"`
	Time        time.Time      `json:"time"`
}

type RunFilter struct {
	EntryID     string
	TemplateKey string
	Status      Status
	Limit       int
}

type RunLogFilter struct {
	RunID   string
	EntryID string
	Limit   int
}

type LogFilter struct {
	RunID   string
	EntryID string
}

type LogStore interface {
	Append(ctx context.Context, event LogEvent) error
	ListRunLogs(ctx context.Context, filter RunLogFilter) ([]LogEvent, error)
}

type LogStreamer interface {
	Subscribe(ctx context.Context, filter LogFilter) (<-chan LogEvent, error)
	Publish(ctx context.Context, event LogEvent) error
}

type MemoryLogStreamer struct {
	mu          sync.RWMutex
	nextID      int64
	subscribers map[int64]memorySubscriber
}

type memorySubscriber struct {
	filter LogFilter
	ch     chan LogEvent
}

func NewMemoryLogStreamer() *MemoryLogStreamer {
	return &MemoryLogStreamer{subscribers: make(map[int64]memorySubscriber)}
}

func (s *MemoryLogStreamer) Subscribe(ctx context.Context, filter LogFilter) (<-chan LogEvent, error) {
	if s == nil {
		s = NewMemoryLogStreamer()
	}
	ch := make(chan LogEvent, 64)

	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.subscribers[id] = memorySubscriber{filter: filter, ch: ch}
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		delete(s.subscribers, id)
		close(ch)
		s.mu.Unlock()
	}()
	return ch, nil
}

func (s *MemoryLogStreamer) Publish(ctx context.Context, event LogEvent) error {
	if s == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sub := range s.subscribers {
		if !matchLogFilter(event, sub.filter) {
			continue
		}
		select {
		case sub.ch <- event:
		default:
		}
	}
	return nil
}

func matchLogFilter(event LogEvent, filter LogFilter) bool {
	if filter.RunID != "" && event.RunID != filter.RunID {
		return false
	}
	if filter.EntryID != "" && event.EntryID != filter.EntryID {
		return false
	}
	return true
}
