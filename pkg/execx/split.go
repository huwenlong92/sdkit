package execx

import (
	"bufio"
	"context"
	"io"
	"time"
)

func readOutput(ctx context.Context, r io.Reader, stream Stream, sink Sink, cfg config) error {
	if r == nil || sink == nil {
		return nil
	}
	switch cfg.splitMode {
	case SplitChunk:
		return readChunks(ctx, r, stream, sink, cfg)
	case SplitCRLF:
		return scanOutput(ctx, r, stream, sink, cfg, scanCRLF)
	default:
		return scanOutput(ctx, r, stream, sink, cfg, bufio.ScanLines)
	}
}

func readChunks(ctx context.Context, r io.Reader, stream Stream, sink Sink, cfg config) error {
	buf := make([]byte, defaultChunkSize)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if writeErr := emit(ctx, sink, stream, buf[:n], cfg); writeErr != nil {
				return writeErr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func scanOutput(ctx context.Context, r io.Reader, stream Stream, sink Sink, cfg config, split bufio.SplitFunc) error {
	scanner := bufio.NewScanner(r)
	scanner.Split(split)
	scanner.Buffer(make([]byte, 0, 64*1024), cfg.lineBufferSize)
	for scanner.Scan() {
		if err := emit(ctx, sink, stream, scanner.Bytes(), cfg); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func emit(ctx context.Context, sink Sink, stream Stream, data []byte, cfg config) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	eventData := make([]byte, len(data))
	copy(eventData, data)
	text, err := cfg.decoder.Decode(eventData)
	if err != nil && cfg.strictDecode {
		return err
	}
	event := Event{
		Stream:    stream,
		Data:      eventData,
		Text:      text,
		Time:      time.Now(),
		DecodeErr: err,
	}
	return sink.WriteCommandEvent(ctx, event)
}

func scanCRLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' || data[i] == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
