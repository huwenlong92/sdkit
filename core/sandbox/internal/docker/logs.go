package docker

import (
	"context"
	"io"
	"sync"

	"github.com/huwenlong92/sdkit/pkg/execx"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

func (r *Runtime) logs(ctx context.Context, containerID string, limit int64) ([]byte, []byte, bool, bool, error) {
	ctx, span := startDockerStep(ctx, "sandbox.logs", nil, "")
	defer span.End()
	body, err := r.client.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return nil, nil, false, false, err
	}
	defer body.Close()
	capture := newLogCapture(limit)
	_, err = stdcopy.StdCopy(streamWriter{ctx: ctx, sink: capture, stream: execx.StreamStdout}, streamWriter{ctx: ctx, sink: capture, stream: execx.StreamStderr}, body)
	stdout, stderr, stdoutTruncated, stderrTruncated := capture.output()
	if err != nil && err != io.EOF {
		return stdout, stderr, stdoutTruncated, stderrTruncated, err
	}
	return stdout, stderr, stdoutTruncated, stderrTruncated, nil
}

type streamWriter struct {
	ctx    context.Context
	sink   execx.Sink
	stream execx.Stream
}

func (w streamWriter) Write(p []byte) (int, error) {
	if err := w.sink.WriteCommandEvent(w.ctx, execx.Event{Stream: w.stream, Data: append([]byte(nil), p...)}); err != nil {
		return 0, err
	}
	return len(p), nil
}

type logCapture struct {
	mu              sync.Mutex
	limit           int64
	stdout          []byte
	stderr          []byte
	stdoutTruncated bool
	stderrTruncated bool
}

func newLogCapture(limit int64) *logCapture {
	if limit < 0 {
		limit = 0
	}
	return &logCapture{limit: limit}
}

func (c *logCapture) WriteCommandEvent(ctx context.Context, event execx.Event) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	switch event.Stream {
	case execx.StreamStderr:
		c.stderr, c.stderrTruncated = appendLimited(c.stderr, event.Data, c.limit, c.stderrTruncated)
	default:
		c.stdout, c.stdoutTruncated = appendLimited(c.stdout, event.Data, c.limit, c.stdoutTruncated)
	}
	return nil
}

func (c *logCapture) output() ([]byte, []byte, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.stdout...), append([]byte(nil), c.stderr...), c.stdoutTruncated, c.stderrTruncated
}

func appendLimited(dst []byte, src []byte, limit int64, truncated bool) ([]byte, bool) {
	if limit == 0 {
		return dst, true
	}
	remaining := int(limit) - len(dst)
	if remaining <= 0 {
		return dst, true
	}
	if len(src) > remaining {
		dst = append(dst, src[:remaining]...)
		return dst, true
	}
	return append(dst, src...), truncated
}
