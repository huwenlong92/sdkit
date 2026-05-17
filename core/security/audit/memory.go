package audit

import (
	"context"
	"sync"
)

type MemoryWriter struct {
	mu     sync.Mutex
	events []*Event
}

func NewMemoryWriter() *MemoryWriter {
	return &MemoryWriter{}
}

func (w *MemoryWriter) Write(ctx context.Context, event *Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if event == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	cp := *event
	w.events = append(w.events, &cp)
	return nil
}

func (w *MemoryWriter) WriteBatch(ctx context.Context, events []*Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, event := range events {
		if err := w.Write(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (w *MemoryWriter) Events() []*Event {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]*Event, len(w.events))
	for i, event := range w.events {
		cp := *event
		out[i] = &cp
	}
	return out
}
