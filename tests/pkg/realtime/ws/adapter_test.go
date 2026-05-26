package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/realtime"
	"github.com/huwenlong92/sdkit/pkg/realtime/ws"

	"github.com/gorilla/websocket"
)

func TestAdapterServesGorillaWebSocket(t *testing.T) {
	registry := newFakeRegistry()
	gotPayload := make(chan []byte, 1)
	adapter := ws.New(registry, ws.Options{
		OnMessage: func(_ context.Context, _ *realtime.Client, payload []byte) error {
			gotPayload <- append([]byte(nil), payload...)
			return nil
		},
	})
	server := httptest.NewServer(adapter)
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL)+"/?user_id=1001", nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	clientID := registry.waitAdded(t)
	client := registry.client(t, clientID)
	client.Ch <- realtime.Event{
		Event:   "notify",
		TraceID: "trace-1",
		Data:    json.RawMessage(`{"ok":true}`),
	}

	var event realtime.Event
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if event.Event != "notify" || event.TraceID != "trace-1" {
		t.Fatalf("event = %+v", event)
	}

	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"ping"}`)); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	select {
	case payload := <-gotPayload:
		if string(payload) != `{"action":"ping"}` {
			t.Fatalf("payload = %s", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for inbound message")
	}

	_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	registry.waitRemoved(t, clientID)
}

func TestAdapterLogsHandlerErrorsWithCommonFields(t *testing.T) {
	registry := newFakeRegistry()
	log := &captureLogger{}
	adapter := ws.New(registry, ws.Options{
		Logger: log,
		OnMessage: func(_ context.Context, _ *realtime.Client, _ []byte) error {
			return errors.New("handler failed")
		},
	})
	server := httptest.NewServer(adapter)
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	registry.waitAdded(t)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"fail"}`)); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	log.waitEntry(t, "websocket message handler failed", "trace_id", "client_id", "err")
}

type fakeRegistry struct {
	mu      sync.Mutex
	added   chan string
	removed chan string
	clients map[string]*realtime.Client
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{
		added:   make(chan string, 4),
		removed: make(chan string, 4),
		clients: make(map[string]*realtime.Client),
	}
}

func (r *fakeRegistry) Add(c *realtime.Client) error {
	r.mu.Lock()
	r.clients[c.ID] = c
	r.mu.Unlock()
	r.added <- c.ID
	return nil
}

func (r *fakeRegistry) Remove(clientID string) error {
	r.mu.Lock()
	delete(r.clients, clientID)
	r.mu.Unlock()
	r.removed <- clientID
	return nil
}

func (r *fakeRegistry) waitAdded(t *testing.T) string {
	t.Helper()
	select {
	case clientID := <-r.added:
		return clientID
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for client add")
	}
	return ""
}

func (r *fakeRegistry) waitRemoved(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-r.removed:
		if got != want {
			t.Fatalf("removed client: want %s, got %s", want, got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for client remove")
	}
}

func (r *fakeRegistry) client(t *testing.T, clientID string) *realtime.Client {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	client := r.clients[clientID]
	if client == nil {
		t.Fatalf("client %s not found", clientID)
	}
	return client
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
		if entry.msg == msg && fieldsContainKeys(entry.fields, keys...) {
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

func wsURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http")
}
