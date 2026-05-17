package crontab

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/tracking"
)

type JobLogger interface {
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
}

type jobLoggerKey struct{}

func WithJobLogger(ctx context.Context, logger JobLogger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, jobLoggerKey{}, logger)
}

func JobLoggerFromContext(ctx context.Context) JobLogger {
	if ctx != nil {
		if logger, ok := ctx.Value(jobLoggerKey{}).(JobLogger); ok && logger != nil {
			return logger
		}
	}
	return NoopJobLogger{}
}

type NoopJobLogger struct{}

func (NoopJobLogger) Info(msg string, fields ...any)  {}
func (NoopJobLogger) Warn(msg string, fields ...any)  {}
func (NoopJobLogger) Error(msg string, fields ...any) {}

type runJobLogger struct {
	ctx         context.Context
	runID       string
	entryID     string
	templateKey string
	store       LogStore
	streamer    LogStreamer
}

func (l *runJobLogger) Info(msg string, fields ...any) {
	l.write(LogInfo, msg, fields...)
}

func (l *runJobLogger) Warn(msg string, fields ...any) {
	l.write(LogWarn, msg, fields...)
}

func (l *runJobLogger) Error(msg string, fields ...any) {
	l.write(LogError, msg, fields...)
}

func (l *runJobLogger) write(level LogLevel, msg string, fields ...any) {
	if l == nil {
		return
	}
	event := LogEvent{
		RunID:       l.runID,
		EntryID:     l.entryID,
		TemplateKey: l.templateKey,
		TrackID:     tracking.TrackID(l.ctx),
		Level:       level,
		Message:     msg,
		Fields:      fieldsToMap(fields...),
		Time:        time.Now(),
	}
	if l.store != nil {
		_ = l.store.Append(l.ctx, event)
	}
	if l.streamer != nil {
		_ = l.streamer.Publish(l.ctx, event)
	}
}

func fieldsToMap(fields ...any) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	if len(fields) == 1 {
		if m, ok := fields[0].(map[string]any); ok {
			return m
		}
	}
	out := make(map[string]any, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if !ok || key == "" {
			continue
		}
		out[key] = fields[i+1]
	}
	return out
}
