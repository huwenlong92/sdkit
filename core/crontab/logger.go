package crontab

import "context"

type LogWriter interface {
	Write(ctx context.Context, log RunLog) error
	WriteBatch(ctx context.Context, logs []RunLog) error
}

type NoopLogWriter struct{}

func (NoopLogWriter) Write(ctx context.Context, log RunLog) error         { return nil }
func (NoopLogWriter) WriteBatch(ctx context.Context, logs []RunLog) error { return nil }
