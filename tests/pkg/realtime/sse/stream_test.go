package tests

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/realtime"
	realtimesse "github.com/huwenlong92/sdkit/pkg/realtime/sse"
)

func TestStreamerWritesEventsAndFlushes(t *testing.T) {
	events := make(chan realtime.Event, 1)
	var buf bytes.Buffer
	flusher := &countFlusher{}
	streamer := realtimesse.NewStreamer(context.Background(), &buf, events, realtimesse.StreamOptions{}).WithFlusher(flusher)
	done := make(chan error, 1)
	go func() {
		done <- streamer.Run()
	}()

	events <- realtime.Event{Event: "notify", TraceID: "trace-1"}
	flusher.wait(t)
	if got := buf.String(); !strings.Contains(got, "event: notify\n") || !strings.Contains(got, `"trace_id":"trace-1"`) {
		t.Fatalf("stream output: %q", got)
	}
	if err := streamer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := streamer.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	waitRunDone(t, done)
}

func TestStreamerWritesHeartbeatAndStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var buf bytes.Buffer
	flusher := &countFlusher{}
	streamer := realtimesse.NewStreamer(ctx, &buf, nil, realtimesse.StreamOptions{
		HeartbeatInterval: time.Millisecond,
	}).WithFlusher(flusher)
	done := make(chan error, 1)
	go func() {
		done <- streamer.Run()
	}()

	flusher.wait(t)
	if got := buf.String(); !strings.Contains(got, ": ping\n\n") {
		t.Fatalf("heartbeat output: %q", got)
	}
	cancel()
	waitRunDone(t, done)
}

func TestStreamerLogsWriteErrorsWithCommonFields(t *testing.T) {
	events := make(chan realtime.Event, 1)
	log := &captureLogger{}
	streamer := realtimesse.NewStreamer(context.Background(), failingWriter{}, events, realtimesse.StreamOptions{
		Logger: log,
	})
	done := make(chan error, 1)
	go func() {
		done <- streamer.Run()
	}()

	events <- realtime.Event{Event: "notify", TraceID: "trace-1"}
	if err := <-done; err == nil {
		t.Fatal("expected stream write error")
	}
	log.waitEntry(t, "sse event write failed", "trace_id", "event", "err")
}

type countFlusher struct {
	mu sync.Mutex
	ch chan struct{}
	n  int
}

func (f *countFlusher) Flush() {
	f.mu.Lock()
	if f.ch == nil {
		f.ch = make(chan struct{}, 16)
	}
	f.n++
	ch := f.ch
	f.mu.Unlock()
	ch <- struct{}{}
}

func (f *countFlusher) wait(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	if f.ch == nil {
		f.ch = make(chan struct{}, 16)
	}
	ch := f.ch
	f.mu.Unlock()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for flush")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func waitRunDone(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for streamer")
	}
}

type logEntry struct {
	msg    string
	fields []any
}

type captureLogger struct {
	mu      sync.Mutex
	entries []logEntry
	notify  chan struct{}
}

func (l *captureLogger) Warn(msg string, fields ...any) {
	l.mu.Lock()
	if l.notify == nil {
		l.notify = make(chan struct{}, 16)
	}
	l.entries = append(l.entries, logEntry{msg: msg, fields: append([]any(nil), fields...)})
	notify := l.notify
	l.mu.Unlock()
	notify <- struct{}{}
}

func (l *captureLogger) waitEntry(t *testing.T, msg string, keys ...string) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if l.hasEntry(msg, keys...) {
			return
		}
		l.mu.Lock()
		notify := l.notify
		if notify == nil {
			notify = make(chan struct{}, 16)
			l.notify = notify
		}
		l.mu.Unlock()
		select {
		case <-notify:
		case <-deadline:
			t.Fatalf("timeout waiting for log %q with keys %v", msg, keys)
		}
	}
}

func (l *captureLogger) hasEntry(msg string, keys ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, entry := range l.entries {
		if entry.msg != msg {
			continue
		}
		if fieldsContainKeys(entry.fields, keys...) {
			return true
		}
	}
	return false
}

func fieldsContainKeys(fields []any, keys ...string) bool {
	found := make(map[string]bool, len(keys))
	for i := 0; i+1 < len(fields); i += 2 {
		key, _ := fields[i].(string)
		if key != "" {
			found[key] = true
		}
	}
	for _, key := range keys {
		if !found[key] {
			return false
		}
	}
	return true
}

var _ io.Writer = failingWriter{}
