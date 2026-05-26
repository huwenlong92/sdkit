package tests

import (
	"bytes"
	"testing"

	"github.com/huwenlong92/sdkit/pkg/realtime/sse"
)

func TestWriteEvent(t *testing.T) {
	var buf bytes.Buffer
	if err := sse.WriteEvent(&buf, "task.update", []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}
	want := "event: task.update\ndata: {\"ok\":true}\n\n"
	if buf.String() != want {
		t.Fatalf("SSE output: want %q, got %q", want, buf.String())
	}
}

func TestWriteComment(t *testing.T) {
	var buf bytes.Buffer
	if err := sse.WriteComment(&buf, "ping"); err != nil {
		t.Fatalf("WriteComment: %v", err)
	}
	if buf.String() != ": ping\n\n" {
		t.Fatalf("comment output: %q", buf.String())
	}
}

func TestWriteEventMultilineData(t *testing.T) {
	var buf bytes.Buffer
	if err := sse.WriteEvent(&buf, "log.line", []byte("a\nb")); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}
	want := "event: log.line\ndata: a\ndata: b\n\n"
	if buf.String() != want {
		t.Fatalf("SSE output: want %q, got %q", want, buf.String())
	}
}
