package tests

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/realtime"
	realtimews "github.com/huwenlong92/sdkit/pkg/realtime/ws"
)

func TestServeConnWritesRealtimeEvents(t *testing.T) {
	registry := newFakeRegistry()
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	client := &realtime.Client{
		ID:     "c1",
		UserID: "1001",
		Events: map[string]bool{"notify": true},
		Ch:     make(chan realtime.Event, 1),
	}
	adapter := realtimews.New(registry, realtimews.Options{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		adapter.ServeConn(context.Background(), realtimews.NewConn(serverConn, nil), client)
	}()

	registry.waitAdded(t, "c1")
	client.Ch <- realtime.Event{
		Event:   "notify",
		TraceID: "trace-1",
		Headers: map[string]string{
			"X-Request-ID":  "request-1",
			"connection_id": "conn-1",
		},
		Data: json.RawMessage(`{"ok":true}`),
	}

	opcode, payload := readServerFrame(t, clientConn)
	if opcode != 1 {
		t.Fatalf("opcode: want text, got %d", opcode)
	}
	var got realtime.Event
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if got.Event != "notify" || got.TraceID != "trace-1" {
		t.Fatalf("event = %+v", got)
	}
	if got.Headers["X-Request-ID"] != "request-1" || got.Headers["connection_id"] != "conn-1" {
		t.Fatalf("headers = %+v", got.Headers)
	}

	writeClientFrame(t, clientConn, 8, nil)
	waitDone(t, done)
	registry.assertRemoved(t, "c1")
}

func TestServeConnDispatchesInboundTextAndRemovesOnClose(t *testing.T) {
	registry := newFakeRegistry()
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	gotPayload := make(chan []byte, 1)
	adapter := realtimews.New(registry, realtimews.Options{
		OnMessage: func(_ context.Context, _ *realtime.Client, payload []byte) error {
			gotPayload <- append([]byte(nil), payload...)
			return nil
		},
	})
	client := &realtime.Client{ID: "c1", Ch: make(chan realtime.Event, 1)}
	done := make(chan struct{})
	go func() {
		defer close(done)
		adapter.ServeConn(context.Background(), realtimews.NewConn(serverConn, nil), client)
	}()

	registry.waitAdded(t, "c1")
	writeClientFrame(t, clientConn, 1, []byte(`{"action":"ping"}`))

	select {
	case payload := <-gotPayload:
		if string(payload) != `{"action":"ping"}` {
			t.Fatalf("payload: %s", string(payload))
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for inbound message")
	}

	writeClientFrame(t, clientConn, 8, nil)
	waitDone(t, done)
	registry.assertRemoved(t, "c1")
}

func TestServeConnReturnsOnContextCancel(t *testing.T) {
	registry := newFakeRegistry()
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	adapter := realtimews.New(registry, realtimews.Options{})
	client := &realtime.Client{ID: "c1", Ch: make(chan realtime.Event, 1)}
	done := make(chan struct{})
	go func() {
		defer close(done)
		adapter.ServeConn(ctx, realtimews.NewConn(serverConn, nil), client)
	}()

	registry.waitAdded(t, "c1")
	cancel()
	waitDone(t, done)
	registry.assertRemoved(t, "c1")
}

func TestServeConnClosesWhenClientChannelCloses(t *testing.T) {
	registry := newFakeRegistry()
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	adapter := realtimews.New(registry, realtimews.Options{})
	client := &realtime.Client{ID: "c1", Ch: make(chan realtime.Event)}
	done := make(chan struct{})
	go func() {
		defer close(done)
		adapter.ServeConn(context.Background(), realtimews.NewConn(serverConn, nil), client)
	}()

	registry.waitAdded(t, "c1")
	close(client.Ch)
	opcode, _ := readServerFrame(t, clientConn)
	if opcode != 8 {
		t.Fatalf("opcode: want close, got %d", opcode)
	}
	waitDone(t, done)
	registry.assertRemoved(t, "c1")
}

func TestServeConnLogsHandlerErrorsWithCommonFields(t *testing.T) {
	registry := newFakeRegistry()
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	log := &captureLogger{}
	adapter := realtimews.New(registry, realtimews.Options{
		Logger: log,
		OnMessage: func(_ context.Context, _ *realtime.Client, _ []byte) error {
			return errors.New("handler failed")
		},
	})
	client := &realtime.Client{ID: "c1", Ch: make(chan realtime.Event, 1)}
	done := make(chan struct{})
	go func() {
		defer close(done)
		adapter.ServeConn(context.Background(), realtimews.NewConn(serverConn, nil), client)
	}()

	registry.waitAdded(t, "c1")
	writeClientFrame(t, clientConn, 1, []byte(`{"action":"fail"}`))
	log.waitEntry(t, "websocket message handler failed", "trace_id", "client_id", "err")

	writeClientFrame(t, clientConn, 8, nil)
	waitDone(t, done)
}

func TestConnWriteTextSupportsExtendedPayloadLengths(t *testing.T) {
	cases := []int{125, 126, 66000}
	for _, size := range cases {
		t.Run(payloadSizeName(size), func(t *testing.T) {
			serverConn, clientConn := net.Pipe()
			defer clientConn.Close()
			conn := realtimews.NewConn(serverConn, nil)
			payload := bytes.Repeat([]byte("x"), size)

			errCh := make(chan error, 1)
			go func() {
				errCh <- conn.WriteText(payload)
			}()

			opcode, got := readServerFrame(t, clientConn)
			if opcode != 1 {
				t.Fatalf("opcode: want text, got %d", opcode)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("payload len: want %d, got %d", len(payload), len(got))
			}
			if err := <-errCh; err != nil {
				t.Fatalf("write text: %v", err)
			}
			_ = serverConn.Close()
		})
	}
}

func TestConnReadTextSupportsMaskedExtendedPayloadLengths(t *testing.T) {
	cases := []int{125, 126, 66000}
	for _, size := range cases {
		t.Run(payloadSizeName(size), func(t *testing.T) {
			serverConn, clientConn := net.Pipe()
			defer clientConn.Close()
			conn := realtimews.NewConn(serverConn, nil)
			payload := bytes.Repeat([]byte("y"), size)

			writeDone := make(chan struct{})
			go func() {
				defer close(writeDone)
				writeClientFrame(t, clientConn, 1, payload)
			}()

			got, opcode, err := conn.ReadText(int64(size + 1))
			if err != nil {
				t.Fatalf("read text: %v", err)
			}
			if opcode != 1 {
				t.Fatalf("opcode: want text, got %d", opcode)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("payload len: want %d, got %d", len(payload), len(got))
			}
			<-writeDone
			_ = serverConn.Close()
		})
	}
}

func TestConnReadTextRejectsOversizedPayload(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	conn := realtimews.NewConn(serverConn, nil)

	go writeClientFrame(t, clientConn, 1, []byte("too-large"))
	if _, _, err := conn.ReadText(3); err == nil {
		t.Fatal("expected oversized payload error")
	}
	_ = serverConn.Close()
}

type fakeRegistry struct {
	mu      sync.Mutex
	added   chan string
	removed chan string
	clients map[string]*realtime.Client
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

func (r *fakeRegistry) waitAdded(t *testing.T, clientID string) {
	t.Helper()
	select {
	case got := <-r.added:
		if got != clientID {
			t.Fatalf("added client: want %s, got %s", clientID, got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for client add")
	}
}

func (r *fakeRegistry) assertRemoved(t *testing.T, clientID string) {
	t.Helper()
	select {
	case got := <-r.removed:
		if got != clientID {
			t.Fatalf("removed client: want %s, got %s", clientID, got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for client remove")
	}
}

func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ServeConn")
	}
}

func payloadSizeName(size int) string {
	switch {
	case size < 126:
		return "small"
	case size <= 65535:
		return "extended16"
	default:
		return "extended64"
	}
}

func readServerFrame(t *testing.T, conn net.Conn) (int, []byte) {
	t.Helper()
	var header [2]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		t.Fatalf("read frame header: %v", err)
	}
	opcode := int(header[0] & 0x0f)
	n := int64(header[1] & 0x7f)
	switch n {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(conn, b[:]); err != nil {
			t.Fatalf("read frame len16: %v", err)
		}
		n = int64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(conn, b[:]); err != nil {
			t.Fatalf("read frame len64: %v", err)
		}
		n = int64(binary.BigEndian.Uint64(b[:]))
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(conn, payload); err != nil {
		t.Fatalf("read frame payload: %v", err)
	}
	return opcode, payload
}

func writeClientFrame(t *testing.T, conn net.Conn, opcode int, payload []byte) {
	t.Helper()
	key := [4]byte{1, 2, 3, 4}
	frame := []byte{0x80 | byte(opcode)}
	switch n := len(payload); {
	case n < 126:
		frame = append(frame, 0x80|byte(n))
	case n <= 65535:
		frame = append(frame, 0x80|126, byte(n>>8), byte(n))
	default:
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		frame = append(frame, 0x80|127)
		frame = append(frame, b[:]...)
	}
	frame = append(frame, key[:]...)
	for i, b := range payload {
		frame = append(frame, b^key[i%4])
	}
	if _, err := conn.Write(frame); err != nil {
		t.Fatalf("write client frame: %v", err)
	}
}
