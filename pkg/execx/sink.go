package execx

import (
	"context"
	"io"
	"sync"
)

type Sink interface {
	WriteCommandEvent(ctx context.Context, event Event) error
}

type SinkFunc func(ctx context.Context, event Event) error

func (f SinkFunc) WriteCommandEvent(ctx context.Context, event Event) error {
	if f == nil {
		return nil
	}
	return f(ctx, event)
}

type MultiSink []Sink

func (s MultiSink) WriteCommandEvent(ctx context.Context, event Event) error {
	for _, sink := range s {
		if sink == nil {
			continue
		}
		if err := sink.WriteCommandEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

type WriterSink struct {
	Writer io.Writer
}

func NewWriterSink(w io.Writer) WriterSink {
	return WriterSink{Writer: w}
}

func (s WriterSink) WriteCommandEvent(ctx context.Context, event Event) error {
	if s.Writer == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if len(event.Data) > 0 {
		_, err := s.Writer.Write(event.Data)
		return err
	}
	_, err := io.WriteString(s.Writer, event.Text)
	return err
}

type RingSink struct {
	mu     sync.Mutex
	limit  int
	events []Event
}

func NewRingSink(limit int) *RingSink {
	if limit < 0 {
		limit = 0
	}
	return &RingSink{limit: limit}
}

func (s *RingSink) WriteCommandEvent(ctx context.Context, event Event) error {
	if s == nil || s.limit == 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, cloneEvent(event))
	if len(s.events) > s.limit {
		copy(s.events, s.events[len(s.events)-s.limit:])
		s.events = s.events[:s.limit]
	}
	return nil
}

func (s *RingSink) Events() []Event {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	events := make([]Event, len(s.events))
	for i, event := range s.events {
		events[i] = cloneEvent(event)
	}
	return events
}

func cloneEvent(event Event) Event {
	if len(event.Data) > 0 {
		data := make([]byte, len(event.Data))
		copy(data, event.Data)
		event.Data = data
	}
	return event
}
