package sse

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/huwenlong92/sdkit/core/realtime"
	"github.com/huwenlong92/sdkit/pkg/realtime/transport"
)

type Flusher interface {
	Flush()
}

type StreamOptions struct {
	HeartbeatInterval time.Duration
	Logger            transport.Logger
}

type Streamer struct {
	ctx     context.Context
	cancel  context.CancelFunc
	writer  io.Writer
	flusher Flusher
	events  <-chan realtime.Event
	opts    StreamOptions
	closer  *transport.Closer
}

func NewStreamer(ctx context.Context, writer io.Writer, events <-chan realtime.Event, opts StreamOptions) *Streamer {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	opts = normalizeStreamOptions(opts)
	streamer := &Streamer{
		ctx:    ctx,
		cancel: cancel,
		writer: writer,
		events: events,
		opts:   opts,
	}
	streamer.closer = transport.NewCloser(cancel, nil)
	return streamer
}

func (s *Streamer) WithFlusher(flusher Flusher) *Streamer {
	if s != nil {
		s.flusher = flusher
	}
	return s
}

func (s *Streamer) Run() error {
	if s == nil || s.writer == nil {
		return nil
	}
	var ticker *time.Ticker
	if s.opts.HeartbeatInterval > 0 {
		ticker = time.NewTicker(s.opts.HeartbeatInterval)
		defer ticker.Stop()
	}
	var tick <-chan time.Time
	if ticker != nil {
		tick = ticker.C
	}
	for {
		select {
		case <-s.ctx.Done():
			return nil
		case <-tick:
			if err := WriteComment(s.writer, "ping"); err != nil {
				transport.Warn(s.ctx, s.opts.Logger, "sse heartbeat write failed", err)
				return err
			}
			s.flush()
		case event, ok := <-s.events:
			if !ok {
				return nil
			}
			payload, err := json.Marshal(event)
			if err != nil {
				transport.WarnTrace(s.opts.Logger, event.TraceID, "sse event encode failed", err, "event", event.Event)
				continue
			}
			if err := WriteEvent(s.writer, event.Event, payload); err != nil {
				transport.WarnTrace(s.opts.Logger, event.TraceID, "sse event write failed", err, "event", event.Event)
				return err
			}
			s.flush()
		}
	}
}

func (s *Streamer) Close() error {
	if s == nil {
		return nil
	}
	return s.closer.Close()
}

func (s *Streamer) flush() {
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

func normalizeStreamOptions(opts StreamOptions) StreamOptions {
	if opts.HeartbeatInterval < 0 {
		opts.HeartbeatInterval = 0
	}
	opts.Logger = transport.NormalizeLogger(opts.Logger)
	return opts
}
